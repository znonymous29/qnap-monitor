package qnap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const authXML = `<?xml version="1.0" encoding="UTF-8" ?>
<QDocRoot>
  <authPassed><![CDATA[1]]></authPassed>
  <authSid><![CDATA[abc123sid]]></authSid>
</QDocRoot>`

const sysinfoXML = `<?xml version="1.0" encoding="UTF-8" ?>
<QDocRoot>
  <authPassed><![CDATA[1]]></authPassed>
  <func>
    <ownContent>
      <root>
        <cpu_usage><![CDATA[23.4 %]]></cpu_usage>
        <total_memory><![CDATA[16384]]></total_memory>
        <free_memory><![CDATA[4096]]></free_memory>
        <sys_tempc><![CDATA[48.0]]></sys_tempc>
      </root>
    </ownContent>
  </func>
</QDocRoot>`

const qsmartXML = `<?xml version="1.0" encoding="UTF-8" ?>
<QDocRoot>
  <authPassed><![CDATA[1]]></authPassed>
  <HDTempWarnT><![CDATA[55]]></HDTempWarnT>
  <HDTempErrT><![CDATA[60]]></HDTempErrT>
  <Disk_Info>
    <entry>
      <Disk_Alias><![CDATA[3.5" SATA HDD 1]]></Disk_Alias>
      <Disk_Status><![CDATA[0]]></Disk_Status>
      <HDNo><![CDATA[0:1]]></HDNo>
      <Vendor><![CDATA[HGST Test]]></Vendor>
      <Health><![CDATA[OK]]></Health>
      <Capacity><![CDATA[7.28 TB]]></Capacity>
      <Temperature><oC><![CDATA[51]]></oC></Temperature>
      <hd_is_ssd><![CDATA[0]]></hd_is_ssd>
      <Model><![CDATA[HUS728T8TALE6L4]]></Model>
      <Serial><![CDATA[VYGSK46M]]></Serial>
      <FirmVersion><![CDATA[V8GNW9G0]]></FirmVersion>
    </entry>
    <entry>
      <Disk_Alias><![CDATA[M.2 PCIe SSD 1]]></Disk_Alias>
      <Disk_Status><![CDATA[0]]></Disk_Status>
      <HDNo><![CDATA[0:5]]></HDNo>
      <Vendor><![CDATA[ZHITAI]]></Vendor>
      <Health><![CDATA[OK]]></Health>
      <Capacity><![CDATA[953.87 GB]]></Capacity>
      <Temperature><oC><![CDATA[47]]></oC></Temperature>
      <hd_is_ssd><![CDATA[1]]></hd_is_ssd>
      <Model><![CDATA[TiPlus7100 1TB]]></Model>
      <Serial><![CDATA[ZTA41T0BA2324509V3]]></Serial>
      <FirmVersion><![CDATA[ZTA22003]]></FirmVersion>
    </entry>
    <entry>
      <Disk_Alias><![CDATA[3.5" SATA 4]]></Disk_Alias>
      <Disk_Status><![CDATA[-5]]></Disk_Status>
      <HDNo><![CDATA[0:4]]></HDNo>
      <Temperature><oC><![CDATA[]]></oC></Temperature>
    </entry>
  </Disk_Info>
</QDocRoot>`

const diskManageXML = `<?xml version="1.0" encoding="UTF-8" ?>
<QDocRoot>
  <authPassed><![CDATA[1]]></authPassed>
  <Volume_Info>
    <row>
      <vol_no>3</vol_no>
      <vol_label><![CDATA[hdd1]]></vol_label>
      <capacity_bytes>7847329153024</capacity_bytes>
      <freesize_bytes>1644021403648</freesize_bytes>
      <used_percent>79 %</used_percent>
      <filesystem_type>EXT4</filesystem_type>
      <raid_level>1</raid_level>
      <vol_mount_path>/share/CACHEDEV3_DATA</vol_mount_path>
    </row>
  </Volume_Info>
  <result>0</result>
</QDocRoot>`

func newMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/cgi-bin/authLogin.cgi", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(authXML))
	})
	mux.HandleFunc("/cgi-bin/management/manaRequest.cgi", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(sysinfoXML))
	})
	mux.HandleFunc("/cgi-bin/disk/qsmart.cgi", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(qsmartXML))
	})
	mux.HandleFunc("/cgi-bin/disk/disk_manage.cgi", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(diskManageXML))
	})
	return httptest.NewServer(mux)
}

func TestFetchAllParsesSystemMetrics(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	c := New(srv.URL, "admin", "pw")
	res, err := c.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	snap := res.Snap
	if snap.CPUUsage < 23.3 || snap.CPUUsage > 23.5 {
		t.Errorf("CPU expected ~23.4, got %v", snap.CPUUsage)
	}
	if snap.MemUsage < 74.9 || snap.MemUsage > 75.1 {
		t.Errorf("Mem expected ~75, got %v", snap.MemUsage)
	}
	if snap.SysTempC != 48.0 {
		t.Errorf("Temp expected 48.0, got %v", snap.SysTempC)
	}
}

func TestFetchAllParsesDiskInfo(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	c := New(srv.URL, "admin", "pw")
	res, err := c.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	// 3 entries but one has DiskStatus=-5 (empty slot), should still be returned
	if len(res.Disks) != 3 {
		t.Fatalf("expected 3 disk entries, got %d", len(res.Disks))
	}
	d1 := res.Disks[0]
	if d1.HDNo != "0:1" {
		t.Errorf("HDNo expected 0:1, got %s", d1.HDNo)
	}
	if d1.TempC != 51 {
		t.Errorf("Temp expected 51, got %d", d1.TempC)
	}
	if d1.IsSSD {
		t.Errorf("expected HDD, got SSD")
	}
	if d1.Model != "HUS728T8TALE6L4" {
		t.Errorf("Model expected HUS728T8TALE6L4, got %s", d1.Model)
	}
	ssd := res.Disks[1]
	if !ssd.IsSSD {
		t.Errorf("expected SSD for 0:5")
	}
	if ssd.TempC != 47 {
		t.Errorf("SSD temp expected 47, got %d", ssd.TempC)
	}
}

func TestFetchAllParsesVolumeDetails(t *testing.T) {
	srv := newMockServer(t)
	defer srv.Close()

	c := New(srv.URL, "admin", "pw")
	res, err := c.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("FetchAll: %v", err)
	}
	if len(res.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(res.Volumes))
	}
	v := res.Volumes[0]
	if v.VolNo != 3 {
		t.Errorf("VolNo expected 3, got %d", v.VolNo)
	}
	if v.Label != "hdd1" {
		t.Errorf("Label expected hdd1, got %s", v.Label)
	}
	if v.CapacityBytes != 7847329153024 {
		t.Errorf("CapacityBytes mismatch: %d", v.CapacityBytes)
	}
	if v.UsedPct < 78.9 || v.UsedPct > 79.1 {
		t.Errorf("UsedPct expected ~79, got %v", v.UsedPct)
	}
}

func TestFetchReloginsOn404(t *testing.T) {
	var sysHits int
	mux := http.NewServeMux()
	mux.HandleFunc("/cgi-bin/authLogin.cgi", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(authXML))
	})
	mux.HandleFunc("/cgi-bin/management/manaRequest.cgi", func(w http.ResponseWriter, r *http.Request) {
		sysHits++
		if sysHits == 1 {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(sysinfoXML))
	})
	mux.HandleFunc("/cgi-bin/disk/qsmart.cgi", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(qsmartXML))
	})
	mux.HandleFunc("/cgi-bin/disk/disk_manage.cgi", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(diskManageXML))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New(srv.URL, "admin", "pw")
	res, err := c.FetchAll(context.Background())
	if err != nil {
		t.Fatalf("expected re-login retry success, got %v", err)
	}
	if res.Snap.SysTempC != 48.0 {
		t.Errorf("retry result wrong: %+v", res.Snap)
	}
	if sysHits != 2 {
		t.Errorf("expected sysinfo to be hit twice (initial 404 + retry), got %d", sysHits)
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1024", 1024},
		{"1.5 GB", int64(1.5 * 1024 * 1024 * 1024)},
		{"2 TB", 2 * 1024 * 1024 * 1024 * 1024},
		{"", 0},
		{"garbage", 0},
	}
	for _, c := range cases {
		got := parseSize(c.in)
		if got != c.want {
			t.Errorf("parseSize(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
