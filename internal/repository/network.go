package repository

import (
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	repo "github.com/diffscope/diffscope-package-manager/packagerepository"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type NetworkPolicy struct {
	PolicyVersion    int      `json:"policyVersion"`
	MaxDownloadBytes int64    `json:"maxDownloadBytes"`
	MaxExpandedBytes int64    `json:"maxExpandedBytes"`
	MaxFileBytes     int64    `json:"maxFileBytes"`
	TimeoutSeconds   int      `json:"timeoutSeconds"`
	AllowedPorts     []string `json:"allowedPorts"`
	MaxRedirects     int      `json:"maxRedirects"`
}

func DefaultPolicy() NetworkPolicy {
	return NetworkPolicy{PolicyVersion: 1, MaxDownloadBytes: 4 << 30, MaxExpandedBytes: 8 << 30, MaxFileBytes: 4 << 30, TimeoutSeconds: 1200, AllowedPorts: []string{"443"}, MaxRedirects: 5}
}
func ReadPolicy(filename string) (NetworkPolicy, error) {
	p := DefaultPolicy()
	if filename == "" {
		return p, nil
	}
	b, err := os.ReadFile(filename)
	if err != nil {
		return p, err
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	err = d.Decode(&p)
	if err != nil {
		return p, err
	}
	if p.PolicyVersion != 1 || p.MaxDownloadBytes <= 0 || p.MaxExpandedBytes <= 0 || p.MaxFileBytes <= 0 || p.TimeoutSeconds <= 0 || p.MaxRedirects < 0 || p.MaxRedirects > 5 || len(p.AllowedPorts) == 0 {
		return p, fmt.Errorf("invalid trusted policy")
	}
	return p, nil
}
func StableURL(raw string) error {
	if err := repo.ValidateURL(raw, false); err != nil {
		return err
	}
	u, _ := url.Parse(raw)
	if strings.Contains(strings.ToLower(u.Path), "/latest/download/") {
		return fmt.Errorf("mutable latest download alias is forbidden")
	}
	for k := range u.Query() {
		key := strings.ToLower(k)
		if strings.Contains(key, "signature") || strings.Contains(key, "token") || strings.HasPrefix(key, "x-amz-") || key == "expires" || key == "se" || key == "sig" {
			return fmt.Errorf("ephemeral or credential URL query is forbidden")
		}
	}
	return nil
}
func publicIP(ip net.IP) bool {
	a, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	a = a.Unmap()
	if !a.IsGlobalUnicast() || a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() {
		return false
	}
	for _, s := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "2001:db8::/32"} {
		if netip.MustParsePrefix(s).Contains(a) {
			return false
		}
	}
	return true
}
func (p NetworkPolicy) validateURL(raw string) error {
	if err := repo.ValidateURL(raw, false); err != nil {
		return fail("DOWNLOAD_POLICY_VIOLATION", "", err)
	}
	u, _ := url.Parse(raw)
	port := u.Port()
	if port == "" {
		port = "443"
	}
	for _, allowed := range p.AllowedPorts {
		if port == allowed {
			return nil
		}
	}
	return fail("DOWNLOAD_POLICY_VIOLATION", "", fmt.Errorf("port not allowed"))
}
func (p NetworkPolicy) Client() *http.Client {
	dial := &net.Dialer{Timeout: 15 * time.Second}
	t := &http.Transport{DisableCompression: true, MaxConnsPerHost: 2, TLSHandshakeTimeout: 15 * time.Second, ResponseHeaderTimeout: 30 * time.Second}
	t.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if !publicIP(ip.IP) {
				return nil, fail("DOWNLOAD_POLICY_VIOLATION", "", fmt.Errorf("nonpublic destination"))
			}
		}
		var last error
		for _, ip := range ips {
			c, e := dial.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if e == nil {
				return c, nil
			}
			last = e
		}
		if last == nil {
			last = fmt.Errorf("no DNS addresses")
		}
		return nil, last
	}
	return &http.Client{Transport: t, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > p.MaxRedirects {
			return fmt.Errorf("redirect budget exceeded")
		}
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		return p.validateURL(req.URL.String())
	}}
}
func safeNetworkError(err error) error {
	var u *url.Error
	if errors.As(err, &u) {
		return u.Err
	}
	return err
}
func (p NetworkPolicy) Download(ctx context.Context, client *http.Client, raw, tmp string) (*os.File, string, error) {
	if err := p.validateURL(raw); err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(p.TimeoutSeconds)*time.Second)
	defer cancel()
	for attempt := 0; attempt < 3; attempt++ {
		requestCtx, stop := context.WithCancel(ctx)
		req, err := http.NewRequestWithContext(requestCtx, "GET", raw, nil)
		if err != nil {
			stop()
			return nil, "", err
		}
		req.Header.Set("Accept-Encoding", "identity")
		resp, err := client.Do(req)
		if err != nil {
			stop()
			err = safeNetworkError(err)
			var policyErr *Error
			if errors.As(err, &policyErr) {
				return nil, "", policyErr
			}
			if attempt < 2 && ctx.Err() == nil {
				continue
			}
			return nil, "", fail("DOWNLOAD_FAILED", "", err)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			stop()
			retryable := resp.StatusCode == 408 || resp.StatusCode == 429 || resp.StatusCode == 500 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504
			if retryable && attempt < 2 {
				wait := time.Second * time.Duration(attempt+1)
				if v := resp.Header.Get("Retry-After"); v != "" {
					if n, e := strconv.ParseInt(v, 10, 64); e == nil && n >= 0 {
						if n > 120 {
							return nil, "", fail("DOWNLOAD_FAILED", "", fmt.Errorf("server cooldown exceeds wait budget"))
						}
						wait = time.Duration(n) * time.Second
					} else if t, e := http.ParseTime(v); e == nil {
						wait = max(0, time.Until(t))
						if wait > 120*time.Second {
							return nil, "", fmt.Errorf("server cooldown exceeds wait budget")
						}
					}
				}
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, "", ctx.Err()
				case <-timer.C:
				}
				continue
			}
			return nil, "", fail("DOWNLOAD_FAILED", "", fmt.Errorf("HTTP %d", resp.StatusCode))
		}
		if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
			resp.Body.Close()
			stop()
			return nil, "", fail("DOWNLOAD_FAILED", "", fmt.Errorf("non-identity representation"))
		}
		if resp.ContentLength > p.MaxDownloadBytes {
			resp.Body.Close()
			stop()
			return nil, "", fail("RESOURCE_LIMIT_EXCEEDED", "", fmt.Errorf("compressed archive exceeds budget"))
		}
		if err = ensureSpace(tmp, max(0, resp.ContentLength)+(512<<20)); err != nil {
			resp.Body.Close()
			stop()
			return nil, "", err
		}
		f, err := os.CreateTemp(tmp, "archive-*.dspk")
		if err != nil {
			resp.Body.Close()
			stop()
			return nil, "", err
		}
		timer := time.AfterFunc(60*time.Second, stop)
		h := sha512.New()
		buf := make([]byte, 128<<10)
		var total int64
		lastCheck := int64(0)
		for {
			n, e := resp.Body.Read(buf)
			if n > 0 {
				timer.Reset(60 * time.Second)
				total += int64(n)
				if total > p.MaxDownloadBytes {
					err = fail("RESOURCE_LIMIT_EXCEEDED", "", fmt.Errorf("actual archive exceeds budget"))
					break
				}
				if _, err = f.Write(buf[:n]); err != nil {
					break
				}
				h.Write(buf[:n])
				if total-lastCheck >= 32<<20 {
					err = ensureSpace(tmp, 512<<20)
					lastCheck = total
					if err != nil {
						break
					}
				}
			}
			if e != nil {
				if e != io.EOF {
					err = e
				}
				break
			}
		}
		timer.Stop()
		resp.Body.Close()
		stop()
		if err == nil && resp.ContentLength >= 0 && total != resp.ContentLength {
			err = io.ErrUnexpectedEOF
		}
		if err == nil {
			if err = f.Sync(); err == nil {
				return f, hex.EncodeToString(h.Sum(nil)), nil
			}
		}
		name := f.Name()
		f.Close()
		os.Remove(name)
		if attempt == 2 || ctx.Err() != nil {
			return nil, "", fail("DOWNLOAD_FAILED", "", err)
		}
	}
	return nil, "", fmt.Errorf("download exhausted")
}
