// Package config loads hgsd's config.json, creating it with defaults on first run.
package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type Config struct {
	Listen       string   `json:"listen"`
	Token        string   `json:"token"`
	DataDir      string   `json:"-"`
	PublicBase   string   `json:"publicBase"`
	Keywords     []string `json:"keywords"`
	RefreshHours int      `json:"refreshHours"`
}

var defaultKeywords = []string{"沙雕", "表情包", "沙雕动画", "搞笑", "无厘头", "爆笑", "穿越", "系统", "修仙", "逆袭", "规则怪谈", "诡异", "末世", "无限流", "游戏", "异能", "升级", "古代", "都市", "校园", "年代", "乡村", "直播", "求生", "玄幻", "重生", "神豪", "皇帝", "大明", "三国", "西游", "熊猫头", "怪谈", "宿舍", "猎人", "家族", "仙门", "师兄", "老六", "整活"}

// Load reads dir/config.json, writing a default one (with a fresh token) if absent.
func Load(dir string) (*Config, error) {
	path := filepath.Join(dir, "config.json")
	c := &Config{}
	raw, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		c.Token = hex.EncodeToString(buf)
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(raw, c); err != nil {
			return nil, err
		}
	}
	if c.Listen == "" {
		c.Listen = ":8686"
	}
	if len(c.Keywords) == 0 {
		c.Keywords = append([]string(nil), defaultKeywords...)
	}
	if c.RefreshHours <= 0 {
		c.RefreshHours = 6
	}
	c.DataDir = dir
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	out, _ := json.MarshalIndent(c, "", "  ")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return nil, err
	}
	return c, nil
}
