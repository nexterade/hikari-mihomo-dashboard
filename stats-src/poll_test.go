package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"path/filepath"
	"testing"
	"time"
)

type fc struct {
	ID        string
	Up, Down  int64
	Src, Proc string
	Chains    []string
	Start     time.Time
}

func serve(t *testing.T, cur *[]fc, totUp, totDown *int64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer s3" {
			w.WriteHeader(401)
			return
		}
		out := map[string]any{"downloadTotal": *totDown, "uploadTotal": *totUp}
		var cs []map[string]any
		for _, c := range *cur {
			if c.Start.IsZero() {
				c.Start = time.Date(2026, 10, 20, 12, 0, 0, 0, time.FixedZone("x", 7*3600))
			}
			cs = append(cs, map[string]any{"id": c.ID, "upload": c.Up, "download": c.Down, "start": c.Start.Format(time.RFC3339Nano),
				"metadata": map[string]any{"sourceIP": c.Src, "process": c.Proc}, "chains": c.Chains})
		}
		out["connections"] = cs
		json.NewEncoder(w).Encode(out)
	}))
}

func TestPoll(t *testing.T) {
	var cur []fc
	var tu, td int64
	srv := serve(t, &cur, &tu, &td)
	defer srv.Close()
	loc, _ := time.LoadLocation("Asia/Jakarta")
	st := &Store{path: filepath.Join(t.TempDir(), "s.json"), loc: loc}
	st.load()
	cls := &Classifier{wifi: []netip.Prefix{netip.MustParsePrefix("192.168.0.0/16")}}
	p := &Poller{api: srv.URL, secret: "s3", cl: srv.Client(), store: st, cls: cls, st: &Status{}, last: map[string]seenConn{}, baseline: true, share: map[string]float64{}}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, loc)
	today := func() *Day { st.mu.Lock(); defer st.mu.Unlock(); return st.day(now) }
	sum := func(net string) Ctr {
		d := today()
		if c := d.Nets[net]; c != nil {
			return *c
		}
		return Ctr{}
	}

	// poll 1: baseline. koneksi lama dengan byte besar TIDAK dihitung
	cur = []fc{{"a", 1000, 5000, "192.168.3.100", "com.x", []string{"yukari", "Inject"}, time.Time{}}}
	tu, td = 1000, 5000
	p.poll(now)
	if c := sum(NetWifi); c.Down != 0 || c.Up != 0 {
		t.Fatalf("baseline harus nol: %+v", c)
	}
	// poll 2: a naik, b baru (data seluler), c baru internal
	cur = []fc{
		{"a", 1100, 5600, "192.168.3.100", "com.x", []string{"yukari", "Inject"}, time.Time{}},
		{"b", 50, 300, "10.20.30.40", "com.y", []string{"Direct", "Smart"}, time.Time{}},
		{"c", 10, 20, "", "", []string{"yukari", "Inject"}, time.Time{}},
	}
	tu, td = 1160, 5920
	p.poll(now)
	if c := sum(NetWifi); c.Up != 100 || c.Down != 600 || c.Conns != 0 {
		t.Fatalf("wifi: %+v", c)
	}
	if c := sum(NetData); c.Up != 50 || c.Down != 300 || c.Conns != 1 {
		t.Fatalf("data: %+v", c)
	}
	if c := sum(NetInternal); c.Up != 10 || c.Down != 20 || c.Conns != 1 {
		t.Fatalf("internal: %+v", c)
	}
	// poll 3: b nutup; 400 byte down terakhir b ikut counter global tapi nggak kelihatan -> dibagi proporsional
	cur = []fc{{"a", 1100, 5700, "192.168.3.100", "com.x", []string{"yukari", "Inject"}, time.Time{}}}
	tu, td = 1160, 5920+100+400
	p.poll(now)
	w, d, in := sum(NetWifi), sum(NetData), sum(NetInternal)
	totalDown := w.Down + d.Down + in.Down
	if totalDown != td-5000 {
		t.Fatalf("total down harus == delta global %d, dapat %d (w=%d d=%d i=%d)", td-5000, totalDown, w.Down, d.Down, in.Down)
	}
	// poll 4: core restart (counter global turun) -> baseline, tidak ada lonjakan
	before := totalDown
	cur = []fc{{ID: "z", Up: 999999, Down: 888888, Src: "192.168.3.100", Proc: "com.x", Chains: []string{"yukari", "Inject"}, Start: now.Add(-5 * time.Minute)}}
	tu, td = 999999, 888888 // lebih BESAR dari counter lama: cuma bisa ketahuan lewat waktu start
	p.poll(now)
	if x := sum(NetWifi).Down + sum(NetData).Down + sum(NetInternal).Down; x != before {
		t.Fatalf("restart core nambah byte: %d != %d", x, before)
	}
	cur[0].Down += 70
	td += 70
	p.poll(now)
	if x := sum(NetWifi).Down; x != w.Down+70+(sum(NetWifi).Down-w.Down-70) {
		t.Fatal("hitung ulang")
	}
	if got := sum(NetWifi).Down - w.Down; got < 70 {
		t.Fatalf("setelah restart harus nambah >=70, dapat %d", got)
	}
	// persist roundtrip
	st.flush()
	st2 := &Store{path: st.path, loc: loc}
	st2.load()
	if st2.f.Days["2026-10-20"] == nil || st2.f.Days["2026-10-20"].Nets[NetData].Down != d.Down {
		t.Fatal("persist gagal")
	}
	// secret salah
	p.secret = "salah"
	p.poll(now)
	if p.st.OK {
		t.Fatal("harusnya gagal")
	}
}

func TestSummaryPeriods(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Jakarta")
	st := &Store{path: filepath.Join(t.TempDir(), "s.json"), loc: loc}
	st.load()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, loc) // Selasa
	for _, ds := range []string{"2026-10-19", "2026-10-20", "2026-10-14", "2026-09-30", "2026-10-01"} {
		d := newDay()
		d.Nets[NetWifi] = &Ctr{Down: 1000}
		d.Nets[NetData] = &Ctr{Down: 10}
		st.f.Days[ds] = d
	}
	cls := &Classifier{}
	s := buildSummary(st, cls, &Status{}, loc, now)
	wk := s.Weekly[len(s.Weekly)-1] // minggu ini: Sen 19 - Min 25
	if wk.Start != "2026-10-19" || wk.Nets[NetWifi].Down != 2000 {
		t.Fatalf("minggu ini: %+v", wk)
	}
	mo := s.Monthly[len(s.Monthly)-1]
	if mo.Key != "2026-10" || mo.Nets[NetWifi].Down != 4000 { // 1,14,19,20 okt
		t.Fatalf("bulan ini: %+v", mo)
	}
	if pm := s.Monthly[len(s.Monthly)-2]; pm.Nets[NetWifi].Down != 1000 {
		t.Fatalf("bulan lalu: %+v", pm)
	}
	if dd := s.Daily[len(s.Daily)-1]; dd.Key != "2026-10-20" || !dd.Active {
		t.Fatalf("hari ini: %+v", dd)
	}
}

func TestRemainderFallbackShare(t *testing.T) {
	var cur []fc
	var tu, td int64
	srv := serve(t, &cur, &tu, &td)
	defer srv.Close()
	loc, _ := time.LoadLocation("Asia/Jakarta")
	st := &Store{path: filepath.Join(t.TempDir(), "s.json"), loc: loc}
	st.load()
	cls := &Classifier{wifi: []netip.Prefix{netip.MustParsePrefix("192.168.0.0/16")}}
	p := &Poller{api: srv.URL, secret: "s3", cl: srv.Client(), store: st, cls: cls, st: &Status{}, last: map[string]seenConn{}, baseline: true, share: map[string]float64{}}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, loc)
	p.poll(now) // baseline kosong
	// ada traffic wifi terlihat
	cur = []fc{{"a", 10, 1000, "192.168.1.5", "x", []string{"n", "Inject"}, time.Time{}}}
	tu, td = 10, 1000
	p.poll(now)
	// koneksi nutup cepat: nggak ada koneksi terlihat, tapi counter global naik 5000
	cur = nil
	td += 5000
	p.poll(now)
	st.mu.Lock()
	d := st.day(now)
	w, in := d.net(NetWifi).Down, d.net(NetInternal).Down
	st.mu.Unlock()
	if w != 6000 || in != 0 {
		t.Fatalf("sisa harus ke wifi (rasio terakhir): wifi=%d internal=%d", w, in)
	}
}
