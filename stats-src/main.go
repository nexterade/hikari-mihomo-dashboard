// hikari-stats: collector statistik trafik Mihomo yang permanen (harian/mingguan/bulanan),
// dipisah WiFi vs Data seluler. Polling /connections, simpan ke JSON, sajikan laporan HTML.
package main

import (
	"context"
	"embed"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"
)

//go:embed ui/index.html
var uiFS embed.FS

const (
	NetWifi     = "wifi"
	NetData     = "data"
	NetInternal = "internal"
)

type Ctr struct {
	Up    int64 `json:"up"`
	Down  int64 `json:"down"`
	Conns int64 `json:"conns"`
}

func (c *Ctr) add(up, down, conns int64) { c.Up += up; c.Down += down; c.Conns += conns }

type Day struct {
	Nets map[string]*Ctr            `json:"nets"`
	Dims map[string]map[string]*Ctr `json:"dims"` // dim -> "net|name" -> ctr
}

func newDay() *Day {
	return &Day{Nets: map[string]*Ctr{}, Dims: map[string]map[string]*Ctr{}}
}

type File struct {
	Version int             `json:"version"`
	Started int64           `json:"started"` // unix detik, pertama kali collector jalan
	Days    map[string]*Day `json:"days"`
}

type Store struct {
	mu    sync.Mutex
	f     File
	dirty bool
	path  string
	loc   *time.Location
}

func (s *Store) day(t time.Time) *Day {
	k := t.In(s.loc).Format("2006-01-02")
	d := s.f.Days[k]
	if d == nil {
		d = newDay()
		s.f.Days[k] = d
	}
	return d
}

func (d *Day) net(n string) *Ctr {
	c := d.Nets[n]
	if c == nil {
		c = &Ctr{}
		d.Nets[n] = c
	}
	return c
}

func (d *Day) dim(dim, net, name string) *Ctr {
	m := d.Dims[dim]
	if m == nil {
		m = map[string]*Ctr{}
		d.Dims[dim] = m
	}
	k := net + "|" + name
	c := m[k]
	if c == nil {
		c = &Ctr{}
		m[k] = c
	}
	return c
}

func (s *Store) load() {
	s.f = File{Version: 1, Days: map[string]*Day{}}
	b, err := os.ReadFile(s.path)
	if err == nil {
		if err := json.Unmarshal(b, &s.f); err != nil {
			bak := s.path + ".corrupt-" + time.Now().Format("20060102-150405")
			_ = os.Rename(s.path, bak)
			log.Printf("file statistik rusak (%v), dipindah ke %s", err, bak)
			s.f = File{Version: 1, Days: map[string]*Day{}}
		}
	}
	if s.f.Days == nil {
		s.f.Days = map[string]*Day{}
	}
	if s.f.Started == 0 {
		s.f.Started = time.Now().Unix()
		s.dirty = true
	}
}

// compact: simpan paling banyak 40 entri teratas per dim/net per hari (sisanya "(lainnya)").
func compactDay(d *Day) {
	const keep = 40
	for dim, m := range d.Dims {
		if len(m) <= keep*3 {
			continue
		}
		byNet := map[string][]string{}
		for k := range m {
			n := k[:strings.Index(k, "|")]
			byNet[n] = append(byNet[n], k)
		}
		for n, keys := range byNet {
			if len(keys) <= keep {
				continue
			}
			sortKeys(keys, m)
			rest := &Ctr{}
			for _, k := range keys[keep:] {
				rest.add(m[k].Up, m[k].Down, m[k].Conns)
				delete(m, k)
			}
			o := n + "|(lainnya)"
			if m[o] == nil {
				m[o] = &Ctr{}
			}
			m[o].add(rest.Up, rest.Down, rest.Conns)
		}
		d.Dims[dim] = m
	}
}

func sortKeys(keys []string, m map[string]*Ctr) {
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && m[keys[j]].Up+m[keys[j]].Down > m[keys[j-1]].Up+m[keys[j-1]].Down; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
}

func (s *Store) flush() {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return
	}
	for _, d := range s.f.Days {
		compactDay(d)
	}
	b, err := json.Marshal(&s.f)
	s.dirty = false
	s.mu.Unlock()
	if err != nil {
		log.Printf("marshal: %v", err)
		return
	}
	if err := os.MkdirAll(dirOf(s.path), 0o755); err != nil {
		log.Printf("mkdir: %v", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		log.Printf("tulis: %v", err)
		s.mu.Lock()
		s.dirty = true
		s.mu.Unlock()
		return
	}
	if err := os.Rename(tmp, s.path); err != nil {
		log.Printf("rename: %v", err)
	}
}

func dirOf(p string) string {
	i := strings.LastIndexByte(p, '/')
	if i <= 0 {
		return "."
	}
	return p[:i]
}

// ---------- klasifikasi ----------

type Classifier struct{ wifi []netip.Prefix }

func (c *Classifier) classify(src string) string {
	if src == "" {
		return NetInternal
	}
	a, err := netip.ParseAddr(src)
	if err != nil {
		return NetInternal
	}
	a = a.Unmap()
	if a.IsLoopback() {
		return NetInternal
	}
	for _, p := range c.wifi {
		if p.Contains(a) {
			return NetWifi
		}
	}
	return NetData
}

func parseWifiFile(path string) []netip.Prefix {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []netip.Prefix
	for _, ln := range strings.Split(string(b), "\n") {
		ln = strings.TrimSpace(ln)
		if strings.HasPrefix(ln, "#") || ln == "" {
			continue
		}
		ln = strings.Trim(ln, `-" `)
		parts := strings.Split(ln, ",")
		if len(parts) >= 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "SRC-IP-CIDR") {
			if p, err := netip.ParsePrefix(strings.Trim(strings.TrimSpace(parts[1]), `"`)); err == nil {
				out = append(out, p)
			}
		}
	}
	return out
}

// ---------- polling ----------

type connsResp struct {
	DownloadTotal int64 `json:"downloadTotal"`
	UploadTotal   int64 `json:"uploadTotal"`
	Connections   []struct {
		ID       string `json:"id"`
		Start    string `json:"start"`
		Upload   int64  `json:"upload"`
		Download int64  `json:"download"`
		Metadata struct {
			SourceIP string `json:"sourceIP"`
			Process  string `json:"process"`
		} `json:"metadata"`
		Chains []string `json:"chains"`
	} `json:"connections"`
}

type Status struct {
	mu       sync.Mutex
	OK       bool   `json:"ok"`
	LastPoll int64  `json:"lastPoll"`
	LastErr  string `json:"lastErr,omitempty"`
	Since    int64  `json:"since"`
}

type seenConn struct{ up, down int64 }

type Poller struct {
	api, secret string
	cl          *http.Client
	store       *Store
	cls         *Classifier
	st          *Status
	last        map[string]seenConn
	baseline    bool // true = poll berikutnya cuma ngeset baseline
	gUp, gDown  int64
	haveG       bool
	lastFetch   time.Time
	share       map[string]float64 // rasio byte per net (rata-rata bergerak), buat bagi sisa kalau nggak ada koneksi terlihat
}

func exitKind(chains []string) string {
	if len(chains) == 0 {
		return "?"
	}
	e := strings.ToLower(chains[0])
	switch {
	case strings.Contains(e, "direct"):
		return "Direct"
	case strings.Contains(e, "reject"):
		return "Reject"
	case strings.Contains(e, "hijack"):
		return "DNS"
	}
	return "Proxy"
}

func (p *Poller) fetch() (*connsResp, error) {
	req, _ := http.NewRequest("GET", p.api+"/connections", nil)
	if p.secret != "" {
		req.Header.Set("Authorization", "Bearer "+p.secret)
	}
	resp, err := p.cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d (secret salah?)", resp.StatusCode)
	}
	var r connsResp
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

func (p *Poller) poll(now time.Time) {
	r, err := p.fetch()
	p.st.mu.Lock()
	if err != nil {
		p.st.OK = false
		p.st.LastErr = err.Error()
		p.st.mu.Unlock()
		return
	}
	p.st.OK, p.st.LastErr, p.st.LastPoll = true, "", now.Unix()
	p.st.mu.Unlock()

	// core restart: counter global turun, ATAU ada koneksi belum dikenal yang mulainya
	// sebelum polling sebelumnya (mustahil kalau core nggak restart).
	restart := p.haveG && (r.DownloadTotal < p.gDown || r.UploadTotal < p.gUp)
	if p.haveG && !p.baseline && !restart {
		for _, c := range r.Connections {
			if _, known := p.last[c.ID]; known {
				continue
			}
			if t, err := time.Parse(time.RFC3339Nano, c.Start); err == nil && t.Before(p.lastFetch.Add(-2*time.Second)) {
				restart = true
				break
			}
		}
	}
	p.lastFetch = now
	if restart {
		p.baseline = true
		p.last = map[string]seenConn{}
	}
	first := p.baseline
	p.baseline = false

	p.store.mu.Lock()
	defer p.store.mu.Unlock()
	day := p.store.day(now)

	type agg struct{ up, down int64 }
	attrib := map[string]*agg{}
	cur := make(map[string]seenConn, len(r.Connections))
	var attUp, attDown int64

	for _, c := range r.Connections {
		cur[c.ID] = seenConn{c.Upload, c.Download}
		if first {
			continue
		}
		prev, known := p.last[c.ID]
		du, dd := c.Upload-prev.up, c.Download-prev.down
		if du < 0 {
			du = 0
		}
		if dd < 0 {
			dd = 0
		}
		var newc int64
		if !known {
			newc = 1
		}
		if du == 0 && dd == 0 && newc == 0 {
			continue
		}
		net := p.cls.classify(c.Metadata.SourceIP)
		day.net(net).Conns += newc
		a := attrib[net]
		if a == nil {
			a = &agg{}
			attrib[net] = a
		}
		a.up += du
		a.down += dd
		attUp += du
		attDown += dd
		day.net(net).Up += du
		day.net(net).Down += dd

		proc := c.Metadata.Process
		if proc == "" {
			proc = "(tanpa app)"
		}
		grp := "?"
		if len(c.Chains) > 0 {
			grp = c.Chains[len(c.Chains)-1]
		}
		day.dim("proc", net, proc).add(du, dd, newc)
		day.dim("group", net, grp).add(du, dd, newc)
		day.dim("exit", net, exitKind(c.Chains)).add(du, dd, newc)
	}

	// sisa dari counter global (koneksi yang nutup di antara dua polling) -> dibagi proporsional ke net
	if !first && p.haveG {
		remUp := (r.UploadTotal - p.gUp) - attUp
		remDown := (r.DownloadTotal - p.gDown) - attDown
		if remUp > 0 || remDown > 0 {
			if remUp < 0 {
				remUp = 0
			}
			if remDown < 0 {
				remDown = 0
			}
			if len(attrib) == 0 {
				// nggak ada koneksi terlihat di polling ini: pakai rasio terakhir (HP biasanya cuma di satu jaringan)
				var sum float64
				for _, v := range p.share {
					sum += v
				}
				if sum <= 0 {
					day.net(NetInternal).Up += remUp
					day.net(NetInternal).Down += remDown
				} else {
					left1, left2 := remUp, remDown
					nets := []string{NetWifi, NetData, NetInternal}
					for i, n := range nets {
						ru := int64(float64(remUp) * p.share[n] / sum)
						rd := int64(float64(remDown) * p.share[n] / sum)
						if i == len(nets)-1 {
							ru, rd = left1, left2
						}
						left1 -= ru
						left2 -= rd
						day.net(n).Up += ru
						day.net(n).Down += rd
					}
				}
			} else {
				var left1, left2 = remUp, remDown
				i := 0
				for net, a := range attrib {
					i++
					var ru, rd int64
					if i == len(attrib) {
						ru, rd = left1, left2
					} else {
						if attUp > 0 {
							ru = remUp * a.up / attUp
						}
						if attDown > 0 {
							rd = remDown * a.down / attDown
						}
					}
					left1 -= ru
					left2 -= rd
					day.net(net).Up += ru
					day.net(net).Down += rd
				}
			}
		}
	}
	for n := range p.share {
		p.share[n] *= 0.97
	}
	for n, a := range attrib {
		p.share[n] += float64(a.up + a.down)
	}
	p.last = cur
	p.gUp, p.gDown, p.haveG = r.UploadTotal, r.DownloadTotal, true
	p.store.dirty = true
}

func main() {
	api := flag.String("api", "http://127.0.0.1:9090", "alamat controller Mihomo")
	secret := flag.String("secret", getenv("HIKARI_SECRET", "hikari"), "secret controller (atau env HIKARI_SECRET)")
	interval := flag.Duration("interval", 3*time.Second, "interval polling")
	listen := flag.String("listen", "127.0.0.1:9099", "alamat halaman laporan")
	data := flag.String("data", "stats.json", "file statistik")
	wifiFile := flag.String("wifi-file", "", "ruleset wifi.yaml (baris SRC-IP-CIDR)")
	wifi := flag.String("wifi", "", "CIDR WiFi tambahan, pisah koma")
	tz := flag.String("tz", "Asia/Jakarta", "zona waktu laporan")
	flag.Parse()

	loc, err := time.LoadLocation(*tz)
	if err != nil {
		log.Fatalf("tz: %v", err)
	}
	cls := &Classifier{}
	if *wifiFile != "" {
		cls.wifi = append(cls.wifi, parseWifiFile(*wifiFile)...)
	}
	for _, s := range strings.Split(*wifi, ",") {
		if s = strings.TrimSpace(s); s != "" {
			if p, err := netip.ParsePrefix(s); err == nil {
				cls.wifi = append(cls.wifi, p)
			} else {
				log.Fatalf("CIDR salah: %q", s)
			}
		}
	}
	if len(cls.wifi) == 0 {
		cls.wifi = []netip.Prefix{netip.MustParsePrefix("192.168.0.0/16")}
	}

	st := &Store{path: *data, loc: loc}
	st.load()
	status := &Status{Since: time.Now().Unix()}
	p := &Poller{api: strings.TrimRight(*api, "/"), secret: *secret, cl: &http.Client{Timeout: 5 * time.Second},
		store: st, cls: cls, st: status, last: map[string]seenConn{}, baseline: true, share: map[string]float64{}}

	srv := &http.Server{Addr: *listen, Handler: newMux(st, cls, status, loc)}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http: %v", err)
		}
	}()
	log.Printf("hikari-stats jalan: laporan http://%s/ | api %s | data %s | wifi %v", *listen, *api, *data, cls.wifi)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	tick := time.NewTicker(*interval)
	fl := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	defer fl.Stop()
	for {
		select {
		case <-ctx.Done():
			st.flush()
			_ = srv.Shutdown(context.Background())
			return
		case t := <-tick.C:
			p.poll(t)
		case <-fl.C:
			st.flush()
		}
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
