import AVFoundation
import UIKit

final class QRScannerPlugin: MyGoPlugin {
    private var scanning = false

    func call(_ method: String, _ args: [String: Any], _ reply: @escaping PluginReply) {
        guard method == "scan" else { return reply(.failure(PluginFailure("NoMethod", method))) }
        guard !scanning else { return reply(.failure(PluginFailure("Busy", "扫码界面已经打开"))) }
        scanning = true
        AVCaptureDevice.requestAccess(for: .video) { granted in
            DispatchQueue.main.async {
                guard granted else {
                    self.scanning = false
                    reply(.failure(PluginFailure("PermissionDenied", "请在系统设置中允许 ChatGo 使用相机")))
                    return
                }
                let vc = ChatGoQRController()
                vc.modalPresentationStyle = .fullScreen
                vc.finished = { text, error in
                    self.scanning = false
                    vc.dismiss(animated: true) {
                        if let error { reply(.failure(PluginFailure("ScanFailed", error))) }
                        else { reply(.success(["text": text])) }
                    }
                }
                PluginHost.present(vc)
            }
        }
    }
}

private final class ChatGoQRController: UIViewController, AVCaptureMetadataOutputObjectsDelegate {
    var finished: ((String, String?) -> Void)?
    private let session = AVCaptureSession()
    private let queue = DispatchQueue(label: "chatgo.qr.camera")
    private var preview: AVCaptureVideoPreviewLayer!
    private var completed = false

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .black
        preview = AVCaptureVideoPreviewLayer(session: session)
        preview.videoGravity = .resizeAspectFill
        view.layer.addSublayer(preview)

        let hint = UILabel()
        hint.text = "扫描桌面 ChatGo 的配对二维码"
        hint.textColor = .white
        hint.textAlignment = .center
        hint.translatesAutoresizingMaskIntoConstraints = false
        view.addSubview(hint)
        let cancel = UIButton(type: .system)
        cancel.setTitle("取消", for: .normal)
        cancel.tintColor = .white
        cancel.addTarget(self, action: #selector(cancelScan), for: .touchUpInside)
        cancel.translatesAutoresizingMaskIntoConstraints = false
        view.addSubview(cancel)
        NSLayoutConstraint.activate([
            cancel.topAnchor.constraint(equalTo: view.safeAreaLayoutGuide.topAnchor, constant: 12),
            cancel.leadingAnchor.constraint(equalTo: view.leadingAnchor, constant: 20),
            cancel.heightAnchor.constraint(equalToConstant: 44),
            hint.centerXAnchor.constraint(equalTo: view.centerXAnchor),
            hint.bottomAnchor.constraint(equalTo: view.safeAreaLayoutGuide.bottomAnchor, constant: -36)
        ])
        queue.async {
            guard let camera = AVCaptureDevice.default(for: .video),
                  let input = try? AVCaptureDeviceInput(device: camera),
                  self.session.canAddInput(input) else {
                DispatchQueue.main.async { self.finish("", "无法打开相机") }
                return
            }
            self.session.beginConfiguration()
            self.session.addInput(input)
            let output = AVCaptureMetadataOutput()
            guard self.session.canAddOutput(output) else {
                self.session.commitConfiguration()
                DispatchQueue.main.async { self.finish("", "相机不支持二维码扫描") }
                return
            }
            self.session.addOutput(output)
            output.setMetadataObjectsDelegate(self, queue: .main)
            output.metadataObjectTypes = [.qr]
            self.session.commitConfiguration()
            self.session.startRunning()
        }
    }

    override func viewDidLayoutSubviews() {
        super.viewDidLayoutSubviews()
        preview.frame = view.bounds
    }

    override func viewDidDisappear(_ animated: Bool) {
        super.viewDidDisappear(animated)
        queue.async { self.session.stopRunning() }
    }

    @objc private func cancelScan() { finish("", nil) }

    private func finish(_ text: String, _ error: String?) {
        guard !completed else { return }
        completed = true
        queue.async { self.session.stopRunning() }
        let callback = finished
        finished = nil
        callback?(text, error)
    }

    func metadataOutput(_ output: AVCaptureMetadataOutput, didOutput objects: [AVMetadataObject], from connection: AVCaptureConnection) {
        if let text = (objects.first as? AVMetadataMachineReadableCodeObject)?.stringValue { finish(text, nil) }
    }
}
