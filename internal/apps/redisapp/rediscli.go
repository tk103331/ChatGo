// Package redisapp is the Apps menu's Redis client: browse keys, view
// values by type, run arbitrary commands and inspect server info through
// the go-redis library.
package redisapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// Conn is one Redis connection.
type Conn struct {
	Host     string
	Port     int
	Password string
	DB       int
}

// Addr returns host:port.
func (c Conn) Addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// KeyMeta is a scanned key plus its type and TTL.
type KeyMeta struct {
	Key   string
	Type  string
	TTL   time.Duration
	Size  int64
}

// ValueView is a key's value rendered for display.
type ValueView struct {
	Type  string
	Text  string
	Lines []string
	Count int
}

// newClient builds a go-redis client for the connection.
func newClient(c Conn) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     c.Addr(),
		Password: c.Password,
		DB:       c.DB,
	})
}

// ping verifies the connection.
func ping(c Conn) error {
	rdb := newClient(c)
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return rdb.Ping(ctx).Err()
}

// scanKeys lists keys matching a pattern.
func scanKeys(c Conn, pattern string, count int64) ([]string, error) {
	rdb := newClient(c)
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if pattern == "" {
		pattern = "*"
	}
	if count <= 0 {
		count = 100
	}
	var keys []string
	var cursor uint64
	for {
		got, next, err := rdb.Scan(ctx, cursor, pattern, count).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, got...)
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return keys, nil
}

// keyMeta inspects one key's type, TTL and size hint.
func keyMeta(c Conn, key string) (KeyMeta, error) {
	rdb := newClient(c)
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	typ, err := rdb.Type(ctx, key).Result()
	if err != nil {
		return KeyMeta{}, err
	}
	ttl, err := rdb.TTL(ctx, key).Result()
	if err != nil {
		return KeyMeta{}, err
	}
	size, err := rdb.StrLen(ctx, key).Result()
	if err != nil {
		size = -1
	}
	return KeyMeta{Key: key, Type: typ, TTL: ttl, Size: size}, nil
}

// keyMetas inspects several keys in one pass.
func keyMetas(c Conn, keys []string) ([]KeyMeta, error) {
	out := make([]KeyMeta, 0, len(keys))
	for _, k := range keys {
		m, err := keyMeta(c, k)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

// getValue reads a key's value according to its type.
func getValue(c Conn, key string, limit int) (ValueView, error) {
	rdb := newClient(c)
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	typ, err := rdb.Type(ctx, key).Result()
	if err != nil {
		return ValueView{}, err
	}
	if limit <= 0 {
		limit = 1000
	}
	switch typ {
	case "string":
		v, err := rdb.Get(ctx, key).Result()
		if err != nil {
			return ValueView{}, err
		}
		lines := splitLines(v, limit)
		return ValueView{Type: typ, Text: v, Lines: lines, Count: len(lines)}, nil
	case "list":
		n, err := rdb.LLen(ctx, key).Result()
		if err != nil {
			return ValueView{}, err
		}
		items, err := rdb.LRange(ctx, key, 0, int64(limit)-1).Result()
		if err != nil {
			return ValueView{}, err
		}
		lines := make([]string, 0, len(items))
		for i, it := range items {
			lines = append(lines, fmt.Sprintf("%d) %s", i, it))
		}
		return ValueView{Type: typ, Text: strings.Join(lines, "\n"), Lines: lines, Count: int(n)}, nil
	case "set":
		items, err := rdb.SMembers(ctx, key).Result()
		if err != nil {
			return ValueView{}, err
		}
		lines := make([]string, 0, len(items))
		for _, it := range items {
			lines = append(lines, it)
		}
		return ValueView{Type: typ, Text: strings.Join(lines, "\n"), Lines: lines, Count: len(lines)}, nil
	case "zset":
		items, err := rdb.ZRangeWithScores(ctx, key, 0, int64(limit)-1).Result()
		if err != nil {
			return ValueView{}, err
		}
		lines := make([]string, 0, len(items))
		for _, it := range items {
			lines = append(lines, fmt.Sprintf("%s\t%g", it.Member, it.Score))
		}
		return ValueView{Type: typ, Text: strings.Join(lines, "\n"), Lines: lines, Count: len(lines)}, nil
	case "hash":
		items, err := rdb.HGetAll(ctx, key).Result()
		if err != nil {
			return ValueView{}, err
		}
		lines := make([]string, 0, len(items))
		for k, v := range items {
			lines = append(lines, fmt.Sprintf("%s:\t%s", k, v))
		}
		return ValueView{Type: typ, Text: strings.Join(lines, "\n"), Lines: lines, Count: len(lines)}, nil
	case "stream":
		items, err := rdb.XRange(ctx, key, "-", "+").Result()
		if err != nil {
			return ValueView{}, err
		}
		if len(items) > limit {
			items = items[:limit]
		}
		lines := make([]string, 0, len(items))
		for _, it := range items {
			fields := make([]string, 0, len(it.Values))
			for fk, fv := range it.Values {
				fields = append(fields, fk+"="+fmt.Sprint(fv))
			}
			lines = append(lines, it.ID+"\t"+strings.Join(fields, " "))
		}
		return ValueView{Type: typ, Text: strings.Join(lines, "\n"), Lines: lines, Count: len(lines)}, nil
	default:
		v, err := rdb.Get(ctx, key).Result()
		if err == nil {
			ll := splitLines(v, limit)
			return ValueView{Type: typ, Text: v, Lines: ll, Count: len(ll)}, nil
		}
		return ValueView{Type: typ, Text: "(无法读取该类型)", Lines: []string{"(无法读取该类型)"}, Count: 1}, nil
	}
}

// deleteKey removes one key.
func deleteKey(c Conn, key string) (int64, error) {
	rdb := newClient(c)
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return rdb.Del(ctx, key).Result()
}

// flushDB empties the selected database.
func flushDB(c Conn) error {
	rdb := newClient(c)
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return rdb.FlushDB(ctx).Err()
}

// dbSize returns the number of keys in the database.
func dbSize(c Conn) (int64, error) {
	rdb := newClient(c)
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return rdb.DBSize(ctx).Result()
}

// info returns an INFO section as raw text.
func info(c Conn, section string) (string, error) {
	rdb := newClient(c)
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if section == "" {
		section = "server"
	}
	return rdb.Info(ctx, section).Result()
}

// runCmd executes an arbitrary command and renders its reply.
func runCmd(c Conn, cmdline string) (string, error) {
	rdb := newClient(c)
	defer rdb.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	parts, err := splitArgs(cmdline)
	if err != nil {
		return "", err
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("命令为空")
	}
	args := make([]any, 0, len(parts))
	for _, p := range parts {
		args = append(args, p)
	}
	res, err := rdb.Do(ctx, args...).Result()
	if err != nil {
		return "", err
	}
	return renderResult(res), nil
}

// renderResult formats a redis command reply as text.
func renderResult(res any) string {
	switch v := res.(type) {
	case nil:
		return "(nil)"
	case string:
		return v
	case int64:
		return fmt.Sprintf("(integer) %d", v)
	case []byte:
		return string(v)
	case []any:
		var b strings.Builder
		for i, item := range v {
			fmt.Fprintf(&b, "%d) %s\n", i+1, renderResult(item))
		}
		return strings.TrimRight(b.String(), "\n")
	case [][]byte:
		var b strings.Builder
		for i, item := range v {
			fmt.Fprintf(&b, "%d) %s\n", i+1, string(item))
		}
		return strings.TrimRight(b.String(), "\n")
	case map[any]any:
		var b strings.Builder
		for k, val := range v {
			fmt.Fprintf(&b, "%s:\t%s\n", fmt.Sprint(k), renderResult(val))
		}
		return strings.TrimRight(b.String(), "\n")
	case float64:
		return fmt.Sprintf("%g", v)
	case bool:
		if v {
			return "(true)"
		}
		return "(false)"
	}
	return fmt.Sprint(res)
}

// splitArgs splits a command line into arguments, honoring quotes.
func splitArgs(line string) ([]string, error) {
	var args []string
	var cur strings.Builder
	inQuote := byte(0)
	for i := 0; i < len(line); i++ {
		ch := line[i]
		switch {
		case inQuote != 0:
			if ch == inQuote {
				inQuote = 0
			} else {
				cur.WriteByte(ch)
			}
		case ch == '\'' || ch == '"':
			inQuote = ch
		case ch == ' ' || ch == '\t':
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteByte(ch)
		}
	}
	if inQuote != 0 {
		return nil, fmt.Errorf("未闭合的引号")
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args, nil
}

// splitLines splits text into at most limit lines.
func splitLines(s string, limit int) []string {
	if s == "" {
		return []string{""}
	}
	lines := strings.Split(s, "\n")
	if len(lines) > limit {
		lines = lines[:limit]
	}
	return lines
}
