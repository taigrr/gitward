package engine

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FetchGitHubKeys downloads a user's public SSH keys from
// https://github.com/<user>.keys and returns the ssh-ed25519 / ssh-rsa lines,
// suitable as age recipients. Network failures are returned so the caller can
// decide whether to proceed.
func FetchGitHubKeys(username string) ([]string, error) {
	url := fmt.Sprintf("https://github.com/%s.keys", username)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	var keys []string
	sc := bufio.NewScanner(strings.NewReader(string(body)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "ssh-ed25519 ") || strings.HasPrefix(line, "ssh-rsa ") {
			keys = append(keys, line)
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("no usable ssh keys at %s", url)
	}
	return keys, nil
}
