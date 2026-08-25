// Package pairing lets a freshly-installed edge agent obtain its
// AGENT_TOKEN without anyone copying a secret into a .env file by hand: it
// serves a small local setup web page, a technician on the same LAN types
// in a short-lived pairing code generated in the central admin dashboard,
// and the agent exchanges that code for the real token itself.
package pairing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

type tokenFile struct {
	AgentToken string `json:"agent_token"`
	SiteID     string `json:"site_id"`
	SiteName   string `json:"site_name"`
}

// LoadOrPair resolves the agent token to use, in priority order:
//  1. envToken (AGENT_TOKEN) — explicit, backward compatible with the
//     original manual .env setup.
//  2. a token already saved at tokenFilePath from a previous pairing.
//  3. pairing mode: block serving a local setup page until someone pairs.
func LoadOrPair(ctx context.Context, apiBaseURL, envToken, tokenFilePath string, port int) (string, error) {
	if envToken != "" {
		return envToken, nil
	}
	if tok, ok := readTokenFile(tokenFilePath); ok {
		return tok, nil
	}
	return runPairingServer(ctx, apiBaseURL, tokenFilePath, port)
}

func readTokenFile(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var tf tokenFile
	if err := json.Unmarshal(data, &tf); err != nil || tf.AgentToken == "" {
		return "", false
	}
	return tf.AgentToken, true
}

func writeTokenFile(path string, tf tokenFile) error {
	data, err := json.MarshalIndent(tf, "", "  ")
	if err != nil {
		return err
	}
	// 0600: this file holds the same live credential as AGENT_TOKEN used to.
	return os.WriteFile(path, data, 0o600)
}

func runPairingServer(ctx context.Context, apiBaseURL, tokenFilePath string, port int) (string, error) {
	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)
	var lastError string

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeForm(w, lastError)
		lastError = ""
	})
	mux.HandleFunc("/pair", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		code := strings.ToUpper(strings.TrimSpace(r.FormValue("code")))
		if code == "" {
			lastError = "Kode pairing wajib diisi."
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		tok, siteID, siteName, err := exchangeCode(apiBaseURL, code)
		if err != nil {
			lastError = err.Error()
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if err := writeTokenFile(tokenFilePath, tokenFile{AgentToken: tok, SiteID: siteID, SiteName: siteName}); err != nil {
			lastError = "Pairing berhasil tapi gagal menyimpan token secara lokal: " + err.Error()
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		writeSuccessPage(w, siteName)
		resultCh <- tok
	})

	srv := &http.Server{Addr: fmt.Sprintf(":%d", port), Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	log.Printf("belum ter-pairing: buka http://<ip-mesin-ini>:%d di browser pada jaringan yang sama, lalu masukkan kode pairing dari Admin -> Sites & Kamera", port)
	for _, ip := range localIPs() {
		log.Printf("  kandidat alamat: http://%s:%d", ip, port)
	}

	// Best-effort: lets a browser reach this machine via a fixed hostname
	// instead of needing one of the IPs above, on networks where mDNS
	// multicast actually reaches across devices. Not fatal if it fails
	// (e.g. no usable local IP yet) — the IP candidates already logged
	// above are always the fallback.
	stopMDNS := func() {}
	if stop, err := advertiseMDNS(port); err != nil {
		log.Printf("mDNS tidak aktif (%v) — pakai salah satu alamat IP di atas", err)
	} else {
		stopMDNS = stop
		log.Printf("  atau: http://nvr-agent.local:%d (kalau jaringan ini mendukung mDNS)", port)
	}

	shutdown := func() {
		stopMDNS()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}

	select {
	case tok := <-resultCh:
		shutdown()
		return tok, nil
	case err := <-errCh:
		return "", err
	case <-ctx.Done():
		shutdown()
		return "", ctx.Err()
	}
}

func exchangeCode(apiBaseURL, code string) (token, siteID, siteName string, err error) {
	body, _ := json.Marshal(map[string]string{"pairing_code": code})
	resp, err := http.Post(apiBaseURL+"/api/agent/pair", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", "", "", fmt.Errorf("gagal menghubungi server pusat: %v", err)
	}
	defer resp.Body.Close()

	var out struct {
		SiteID     string `json:"site_id"`
		SiteName   string `json:"site_name"`
		AgentToken string `json:"agent_token"`
		Error      string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", "", fmt.Errorf("respons server tidak valid")
	}
	if resp.StatusCode != http.StatusOK {
		if out.Error != "" {
			return "", "", "", fmt.Errorf("%s", out.Error)
		}
		return "", "", "", fmt.Errorf("pairing gagal (status %d)", resp.StatusCode)
	}
	return out.AgentToken, out.SiteID, out.SiteName, nil
}

func localIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ips
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			ips = append(ips, ip4.String())
		}
	}
	return ips
}

func writeForm(w http.ResponseWriter, errMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	errHTML := ""
	if errMsg != "" {
		errHTML = fmt.Sprintf(`<p class="err">%s</p>`, html.EscapeString(errMsg))
	}
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="id"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Setup Edge Agent</title>
<style>
body{font-family:system-ui,sans-serif;background:#f8fafc;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0}
form{background:#fff;border:1px solid #e2e8f0;border-radius:8px;padding:32px;max-width:360px;width:100%%;box-sizing:border-box}
h1{font-size:18px;margin:0 0 8px;color:#0f172a}
p.desc{color:#64748b;font-size:14px;margin:0 0 20px}
p.err{color:#dc2626;font-size:13px;margin:0 0 12px}
input{width:100%%;box-sizing:border-box;border:1px solid #cbd5e1;border-radius:6px;padding:10px 12px;font-size:16px;letter-spacing:2px;text-transform:uppercase;margin-bottom:12px}
button{width:100%%;background:#0f172a;color:#fff;border:0;border-radius:6px;padding:10px;font-size:14px;font-weight:600;cursor:pointer}
</style></head>
<body>
<form method="POST" action="/pair">
<h1>Setup Edge Agent</h1>
<p class="desc">Masukkan kode pairing yang ditampilkan di Admin &rarr; Sites &amp; Kamera pada dashboard pusat.</p>
%s
<input name="code" placeholder="KODE PAIRING" autofocus autocomplete="off">
<button type="submit">Hubungkan</button>
</form>
</body></html>`, errHTML)
}

func writeSuccessPage(w http.ResponseWriter, siteName string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="id"><head><meta charset="utf-8"><title>Setup Edge Agent</title>
<style>
body{font-family:system-ui,sans-serif;background:#f8fafc;display:flex;min-height:100vh;align-items:center;justify-content:center;margin:0}
div{background:#fff;border:1px solid #e2e8f0;border-radius:8px;padding:32px;max-width:360px;text-align:center}
h1{font-size:18px;color:#15803d;margin:0 0 8px}
p{color:#64748b;font-size:14px;margin:0}
</style></head>
<body><div><h1>Berhasil terhubung ke &quot;%s&quot;</h1><p>Edge agent mulai berjalan sekarang. Halaman ini boleh ditutup.</p></div></body></html>`, html.EscapeString(siteName))
}
