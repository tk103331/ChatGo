package redisapp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	mcore "git.zacharyzhang.com/ZacharyZhang-NY/MujicaUI/core"
)

// Param is one tool parameter.
type Param struct {
	Desc     string
	Required bool
}

// Tool is one operation exposed to the Agent.
type Tool struct {
	Name   string
	Desc   string
	Params map[string]Param
	Run    func(args map[string]any) (string, error)
}

// Tools returns the operations the Agent may run.
func (cl *Client) Tools() []Tool {
	return []Tool{
		{
			Name: "redis_connect",
			Desc: "连接 Redis 服务器。host 可含端口，如 localhost:6379；db 默认 0",
			Params: map[string]Param{
				"host": {Desc: "主机，可带端口", Required: true},
				"password": {Desc: "密码，可省略"},
				"db":       {Desc: "数据库编号，默认 0"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolConnect(strArg(args, "host"), strArg(args, "password"), strArg(args, "db"))
			},
		},
		{
			Name: "redis_keys",
			Desc: "按模式扫描键，列出键、类型和 TTL",
			Params: map[string]Param{
				"pattern": {Desc: "匹配模式，默认 *"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolKeys(strArg(args, "pattern"))
			},
		},
		{
			Name: "redis_get",
			Desc: "读取一个键的值，自动按类型渲染（string/list/set/zset/hash/stream）",
			Params: map[string]Param{
				"key": {Desc: "键名", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolGet(strArg(args, "key"))
			},
		},
		{
			Name: "redis_ttl",
			Desc: "查看键的过期时间（秒，-1 表示永不过期）",
			Params: map[string]Param{
				"key": {Desc: "键名", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolTTL(strArg(args, "key"))
			},
		},
		{
			Name: "redis_type",
			Desc: "查看键的数据类型",
			Params: map[string]Param{
				"key": {Desc: "键名", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolType(strArg(args, "key"))
			},
		},
		{
			Name: "redis_del",
			Desc: "删除一个键（破坏性操作，先确认）",
			Params: map[string]Param{
				"key": {Desc: "键名", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolDel(strArg(args, "key"))
			},
		},
		{
			Name: "redis_cmd",
			Desc: "执行任意 Redis 命令，参数用引号包裹可含空格",
			Params: map[string]Param{
				"cmd": {Desc: "命令及参数，如 SET foo bar 或 INFO all", Required: true},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolCmd(strArg(args, "cmd"))
			},
		},
		{
			Name: "redis_info",
			Desc: "查看服务器信息分区：server/clients/memory/stats/replication/all",
			Params: map[string]Param{
				"section": {Desc: "分区，默认 server"},
			},
			Run: func(args map[string]any) (string, error) {
				return cl.toolInfo(strArg(args, "section"))
			},
		},
	}
}

// conn returns the current connection or a default one.
func (cl *Client) conn() Conn {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	return cl.st.conn
}

// toolConnect connects to a Redis server.
func (cl *Client) toolConnect(host, password, db string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", fmt.Errorf("缺少必填参数 host")
	}
	h, port := "localhost", 6379
	if i := strings.LastIndex(host, ":"); i >= 0 {
		h = host[:i]
		if p, err := strconv.Atoi(host[i+1:]); err == nil {
			port = p
		}
	} else {
		h = host
	}
	conn := Conn{Host: h, Port: port, Password: password}
	if v, err := strconv.Atoi(strings.TrimSpace(db)); err == nil {
		conn.DB = v
	}
	if err := ping(conn); err != nil {
		return "", fmt.Errorf("连接失败: %v", err)
	}
	cl.mu.Lock()
	cl.st.conn = conn
	cl.st.connected = true
	cl.st.err = ""
	cl.mu.Unlock()
	cl.host.Post(func() {
		cl.host.Toast("已连接 "+conn.Addr()+" DB"+strconv.Itoa(conn.DB), mcore.SeveritySuccess)
		cl.loadKeys()
	})
	return "已连接 " + conn.Addr() + " DB" + strconv.Itoa(conn.DB), nil
}

// toolKeys scans keys matching a pattern.
func (cl *Client) toolKeys(pattern string) (string, error) {
	conn := cl.conn()
	keys, err := scanKeys(conn, pattern, 200)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "(没有匹配的键)", nil
	}
	metas, err := keyMetas(conn, keys)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "(共 %d 个键)\n", len(metas))
	for _, m := range metas {
		fmt.Fprintf(&b, "- %s  %s  TTL=%s\n", m.Key, m.Type, fmtTTL(m.TTL))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// toolGet reads a key's value.
func (cl *Client) toolGet(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("缺少必填参数 key")
	}
	v, err := getValue(cl.conn(), key, 2000)
	if err != nil {
		return "", err
	}
	cl.host.Post(func() {
		cl.mu.Lock()
		cl.st.value = v
		cl.st.valueKey = key
		cl.mu.Unlock()
		cl.host.Invalidate()
	})
	if v.Count == 0 {
		return "(空)", nil
	}
	return fmt.Sprintf("类型 %s，%d 项:\n%s", v.Type, v.Count, v.Text), nil
}

// toolTTL returns a key's TTL.
func (cl *Client) toolTTL(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("缺少必填参数 key")
	}
	m, err := keyMeta(cl.conn(), key)
	if err != nil {
		return "", err
	}
	secs := int64(m.TTL.Seconds())
	if m.TTL < 0 {
		return key + " 永不过期 (-1)", nil
	}
	return fmt.Sprintf("%s TTL=%ds", key, secs), nil
}

// toolType returns a key's type.
func (cl *Client) toolType(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("缺少必填参数 key")
	}
	m, err := keyMeta(cl.conn(), key)
	if err != nil {
		return "", err
	}
	return key + " 类型: " + m.Type, nil
}

// toolDel deletes a key.
func (cl *Client) toolDel(key string) (string, error) {
	if key == "" {
		return "", fmt.Errorf("缺少必填参数 key")
	}
	n, err := deleteKey(cl.conn(), key)
	if err != nil {
		return "", err
	}
	cl.host.Post(func() {
		cl.mu.Lock()
		cl.st.value = ValueView{}
		cl.st.valueKey = ""
		cl.mu.Unlock()
		cl.host.Toast(fmt.Sprintf("已删除 %s（%d 个键）", key, n), mcore.SeveritySuccess)
		cl.loadKeys()
	})
	if n == 0 {
		return "键 " + key + " 不存在", nil
	}
	return "已删除键 " + key, nil
}

// toolCmd runs an arbitrary command.
func (cl *Client) toolCmd(cmd string) (string, error) {
	if strings.TrimSpace(cmd) == "" {
		return "", fmt.Errorf("缺少必填参数 cmd")
	}
	out, err := runCmd(cl.conn(), cmd)
	if err != nil {
		return "", err
	}
	cl.host.Post(func() {
		cl.mu.Lock()
		cl.st.cmdOut = out
		cl.mu.Unlock()
		cl.host.Invalidate()
	})
	return out, nil
}

// toolInfo returns a server info section.
func (cl *Client) toolInfo(section string) (string, error) {
	if section == "" {
		section = "server"
	}
	text, err := info(cl.conn(), section)
	if err != nil {
		return "", err
	}
	cl.host.Post(func() {
		cl.mu.Lock()
		cl.st.infoText = text
		cl.mu.Unlock()
		cl.host.Invalidate()
	})
	return text, nil
}

// strArg reads a string argument.
func strArg(args map[string]any, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
		if b, err := json.Marshal(v); err == nil {
			return strings.Trim(string(b), "\"")
		}
	}
	return ""
}
