// Command tokens 直接为 bench 用户签发 access token。
// 压测时避免每个 worker 调登录接口触发网关限流。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"ClaranAIM/pkg/jwt"
)

type manifest struct {
	Users []struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
	} `json:"users"`
}

func main() {
	manifestPath := flag.String("manifest", "bench/results/seed/manifest.json", "seed manifest")
	out := flag.String("out", "bench/results/seed/tokens.txt", "output token file")
	secret := flag.String("secret", "", "JWT secret (or JWT_SECRET_KEY env)")
	hours := flag.Int64("hours", 24, "token validity hours")
	flag.Parse()

	if strings.TrimSpace(*secret) == "" {
		*secret = os.Getenv("JWT_SECRET_KEY")
	}
	if strings.TrimSpace(*secret) == "" {
		log.Fatal("missing -secret or JWT_SECRET_KEY")
	}
	raw, err := os.ReadFile(*manifestPath)
	if err != nil {
		log.Fatalf("read manifest: %v", err)
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		log.Fatalf("parse manifest: %v", err)
	}
	if len(m.Users) == 0 {
		log.Fatal("manifest has no users")
	}
	lines := make([]string, 0, len(m.Users))
	for _, u := range m.Users {
		token, err := jwt.GenerateAccessToken(*secret, u.ID, u.Username, jwt.RoleUser, *hours)
		if err != nil {
			log.Fatalf("sign token: %v", err)
		}
		lines = append(lines, token)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(*out, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		log.Fatalf("write tokens: %v", err)
	}
	fmt.Printf("wrote %d tokens to %s\n", len(lines), *out)
}
