package mailapp

import (
	"html"
	"strings"
)

const mailDocHead = `<!DOCTYPE html><html><head><meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="default-src 'none'; img-src data: cid:; style-src 'unsafe-inline'; font-src data:;">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
html,body{margin:0;padding:0;background:transparent;}
body{padding:12px 16px;font:13px/1.6 -apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#222;word-break:break-word;overflow-wrap:anywhere;}
img{max-width:100%;height:auto;}
table{max-width:100%;border-collapse:collapse;}
pre{white-space:pre-wrap;overflow-wrap:anywhere;}
a{color:#7d2034;}
blockquote{margin:8px 0;padding-left:12px;border-left:3px solid #ddd;color:#555;}
</style></head><body>`

const mailDocTail = `</body></html>`

// document wraps message HTML with a CSP and base styling so scripts and
// remote resources stay inert.
func document(mailHTML string) string {
	return mailDocHead + mailHTML + mailDocTail
}

// plainDocument renders plain text as a preformatted document.
func plainDocument(text string) string {
	return document("<pre>" + html.EscapeString(text) + "</pre>")
}

// htmlAvailable reports whether a body has HTML content to render.
func htmlAvailable(body MessageBody) bool {
	return strings.TrimSpace(body.HTML) != ""
}
