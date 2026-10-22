package ubus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Client 抽象 ubus 调用，便于单测 mock；默认实现通过 `ubus` CLI（OpenWrt 自带）。
// 热路径避免高频 fork：仅在 scrape 时调用，默认间隔 15s+。
type Client interface {
	// List 返回匹配 pattern 的对象名（如 hostapd.*）。
	List(ctx context.Context, pattern string) ([]string, error)
	// Call 调用 path method，将 JSON 结果解码到 dest。
	Call(ctx context.Context, path, method string, dest any) error
}

// ExecClient 使用本机 `ubus` 可执行文件。
type ExecClient struct {
	Bin     string
	Timeout time.Duration
}

func NewExecClient() *ExecClient {
	return &ExecClient{Bin: "ubus", Timeout: 3 * time.Second}
}

func (c *ExecClient) List(ctx context.Context, pattern string) ([]string, error) {
	out, err := c.run(ctx, "list", pattern)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// `ubus list` 输出每行一个对象名，或 `name => { ... }`
		name := strings.Fields(line)[0]
		names = append(names, name)
	}
	return names, nil
}

func (c *ExecClient) Call(ctx context.Context, path, method string, dest any) error {
	out, err := c.run(ctx, "call", path, method)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(out)) == 0 {
		return fmt.Errorf("ubus call %s %s: empty response", path, method)
	}
	return json.Unmarshal(out, dest)
}

func (c *ExecClient) run(ctx context.Context, args ...string) ([]byte, error) {
	bin := c.Bin
	if bin == "" {
		bin = "ubus"
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("ubus %v: %s", args, msg)
	}
	return stdout.Bytes(), nil
}
