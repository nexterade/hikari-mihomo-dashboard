package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Entry struct {
	Key    string          `json:"key"`
	Label  string          `json:"label"`
	Start  string          `json:"start"`
	End    string          `json:"end"`
	Nets   map[string]Ctr  `json:"nets"`
	Top    map[string][]Tp `json:"top"`
	Active bool            `json:"active"` // periode berjalan
}

type Tp struct {
	Name string `json:"name"`
	Wifi int64  `json:"wifi"`
	Data int64  `json:"data"`
}

type Summary struct {
	Generated int64          `json:"generated"`
	TZ        string         `json:"tz"`
	Wifi      []string       `json:"wifiCidrs"`
	Status    *Status        `json:"status"`
	Started   int64          `json:"started"`
	Daily     []Entry        `json:"daily"`
	Weekly    []Entry        `json:"weekly"`
	Monthly   []Entry        `json:"monthly"`
	Total     map[string]Ctr `json:"total"`
}

func newMux(s *Store, cls *Classifier, st *Status, loc *time.Location) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		b, _ := uiFS.ReadFile("ui/index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(b)
	})
	mux.HandleFunc("/api/summary", func(w http.ResponseWriter, r *http.Request) {
		sum := buildSummary(s, cls, st, loc, time.Now())
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(sum)
	})
	mux.HandleFunc("/api/export.csv", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		days := make([]string, 0, len(s.f.Days))
		for k := range s.f.Days {
			days = append(days, k)
		}
		sort.Strings(days)
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="hikari-stats.csv"`)
		cw := csv.NewWriter(w)
		cw.Write([]string{"tanggal", "jaringan", "upload_bytes", "download_bytes", "koneksi"})
		for _, d := range days {
			for _, n := range []string{NetWifi, NetData, NetInternal} {
				if c := s.f.Days[d].Nets[n]; c != nil {
					cw.Write([]string{d, n, strconv.FormatInt(c.Up, 10), strconv.FormatInt(c.Down, 10), strconv.FormatInt(c.Conns, 10)})
				}
			}
		}
		s.mu.Unlock()
		cw.Flush()
	})
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "ok") })
	return mux
}

func weekStart(t time.Time) time.Time {
	d := int(t.Weekday()+6) % 7 // Senin = 0
	return time.Date(t.Year(), t.Month(), t.Day()-d, 0, 0, 0, 0, t.Location())
}

func buildSummary(s *Store, cls *Classifier, st *Status, loc *time.Location, now time.Time) *Summary {
	now = now.In(loc)
	s.mu.Lock()
	defer s.mu.Unlock()

	day0 := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	mk := func(from, to time.Time, key, label string, active bool) Entry {
		e := Entry{Key: key, Label: label, Start: from.Format("2006-01-02"), End: to.Format("2006-01-02"),
			Nets: map[string]Ctr{}, Top: map[string][]Tp{}, Active: active}
		type k2 struct{ dim, name string }
		acc := map[k2]*Tp{}
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			dd := s.f.Days[d.Format("2006-01-02")]
			if dd == nil {
				continue
			}
			for n, c := range dd.Nets {
				x := e.Nets[n]
				x.add(c.Up, c.Down, c.Conns)
				e.Nets[n] = x
			}
			for dim, m := range dd.Dims {
				for key, c := range m {
					i := strings.Index(key, "|")
					net, name := key[:i], key[i+1:]
					if net == NetInternal {
						continue
					}
					t := acc[k2{dim, name}]
					if t == nil {
						t = &Tp{Name: name}
						acc[k2{dim, name}] = t
					}
					if net == NetWifi {
						t.Wifi += c.Up + c.Down
					} else {
						t.Data += c.Up + c.Down
					}
				}
			}
		}
		for k, t := range acc {
			e.Top[k.dim] = append(e.Top[k.dim], *t)
		}
		for dim := range e.Top {
			l := e.Top[dim]
			sort.Slice(l, func(i, j int) bool { return l[i].Wifi+l[i].Data > l[j].Wifi+l[j].Data })
			if len(l) > 8 {
				l = l[:8]
			}
			e.Top[dim] = l
		}
		return e
	}

	sum := &Summary{Generated: now.Unix(), TZ: loc.String(), Status: st, Started: s.f.Started, Total: map[string]Ctr{}}
	for _, p := range cls.wifi {
		sum.Wifi = append(sum.Wifi, p.String())
	}
	for _, d := range s.f.Days {
		for n, c := range d.Nets {
			x := sum.Total[n]
			x.add(c.Up, c.Down, c.Conns)
			sum.Total[n] = x
		}
	}
	hari := []string{"Min", "Sen", "Sel", "Rab", "Kam", "Jum", "Sab"}
	bulan := []string{"", "Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}
	for i := 29; i >= 0; i-- {
		d := day0.AddDate(0, 0, -i)
		lbl := fmt.Sprintf("%s %d %s", hari[d.Weekday()], d.Day(), bulan[d.Month()])
		sum.Daily = append(sum.Daily, mk(d, d, d.Format("2006-01-02"), lbl, i == 0))
	}
	ws := weekStart(day0)
	for i := 11; i >= 0; i-- {
		a := ws.AddDate(0, 0, -7*i)
		b := a.AddDate(0, 0, 6)
		lbl := fmt.Sprintf("%d %s – %d %s", a.Day(), bulan[a.Month()], b.Day(), bulan[b.Month()])
		sum.Weekly = append(sum.Weekly, mk(a, b, a.Format("2006-01-02"), lbl, i == 0))
	}
	m0 := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	for i := 11; i >= 0; i-- {
		a := m0.AddDate(0, -i, 0)
		b := a.AddDate(0, 1, -1)
		sum.Monthly = append(sum.Monthly, mk(a, b, a.Format("2006-01"), fmt.Sprintf("%s %d", bulan[a.Month()], a.Year()), i == 0))
	}
	return sum
}

func (c *Ctr) addTo(o Ctr) { c.Up += o.Up; c.Down += o.Down; c.Conns += o.Conns }
