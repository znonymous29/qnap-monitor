package qnap

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Snapshot is the unified status read from QNAP.
type Snapshot struct {
	CPUUsage         float64
	MemUsage         float64
	SysTempC         float64
	CPUTempC         float64
	FanRPM           int
	UptimeSeconds    int64
	Model            string
	SerialNumber     string
	FirmwareVersion  string
	VolumeTotalBytes int64
	VolumeUsedBytes  int64
}

// DiskInfo holds per-disk S.M.A.R.T. data from qsmart.cgi.
type DiskInfo struct {
	HDNo              string
	Alias             string
	Model             string
	Serial            string
	Firmware          string
	Vendor            string
	Capacity          string
	CapacityBytes     int64
	Health            string
	TempC             int
	IsSSD             bool
	DiskStatus        int
	PowerOnHours      int64
	ReallocatedSectors int64
}

// VolumeDetail holds per-volume info from disk_manage.cgi.
type VolumeDetail struct {
	VolNo         int
	Label         string
	CapacityBytes int64
	UsedBytes     int64
	FreeBytes     int64
	UsedPct       float64
	Filesystem    string
	RaidLevel     int
	MountPath     string
	HDList        string // e.g. "0000:0001" — maps to disk HDNo "0:1"
}

// FetchResult holds all data from a single collection cycle.
type FetchResult struct {
	Snap         *Snapshot
	Disks        []DiskInfo
	Volumes      []VolumeDetail
	HDTempWarn   int // QNAP native HDD temp warning threshold (°C)
	HDTempErr    int // QNAP native HDD temp error threshold (°C)
	SSDTempWarn  int // QNAP native SSD temp warning threshold (°C)
	SSDTempErr   int // QNAP native SSD temp error threshold (°C)
}

type Client struct {
	BaseURL  string
	User     string
	Password string
	HTTP     *http.Client

	mu  sync.Mutex
	sid string
}

func New(baseURL, user, password string) *Client {
	tr := &http.Transport{
		ResponseHeaderTimeout: 25 * time.Second,
	}
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		User:     user,
		Password: password,
		HTTP:     &http.Client{Timeout: 30 * time.Second, Transport: tr},
	}
}

// --- XML response types ----------------------------------------------------

type authResp struct {
	XMLName    xml.Name `xml:"QDocRoot"`
	AuthSid    string   `xml:"authSid"`
	AuthPassed string   `xml:"authPassed"`
}

type sysinfoResp struct {
	XMLName       xml.Name `xml:"QDocRoot"`
	AuthPassed    string   `xml:"authPassed"`
	CPUUsage      string   `xml:"func>ownContent>root>cpu_usage"`
	TotalMemory   string   `xml:"func>ownContent>root>total_memory"`
	FreeMemory    string   `xml:"func>ownContent>root>free_memory"`
	SysTempC      string   `xml:"func>ownContent>root>sys_tempc"`
	CPUTempC      string   `xml:"func>ownContent>root>cpu_tempc"`
	SystemFan     string   `xml:"func>ownContent>root>system_fan"`
	Uptime        string   `xml:"func>ownContent>root>uptime"`
	Model         string   `xml:"func>ownContent>root>model"`
	SerialNumber  string   `xml:"func>ownContent>root>serial_number"`
	FirmwareVer   string   `xml:"func>ownContent>root>firmware_version"`
}

// qsmartResp is the response from /cgi-bin/disk/qsmart.cgi
type qsmartResp struct {
	XMLName    xml.Name `xml:"QDocRoot"`
	AuthPassed string   `xml:"authPassed"`
	HDTempWarn string   `xml:"HDTempWarnT"`
	HDTempErr  string   `xml:"HDTempErrT"`
	SSDTempWarn string  `xml:"SSDTempWarnT"`
	SSDTempErr  string  `xml:"SSDTempErrT"`
	DiskInfo   struct {
		Entries []diskEntry `xml:"entry"`
	} `xml:"Disk_Info"`
}

type diskEntry struct {
	DiskAlias   string `xml:"Disk_Alias"`
	DiskStatus  string `xml:"Disk_Status"`
	HDNo        string `xml:"HDNo"`
	Vendor      string `xml:"Vendor"`
	Health      string `xml:"Health"`
	Capacity    string `xml:"Capacity"`
	TempC       string `xml:"Temperature>oC"`
	IsSSD       string `xml:"hd_is_ssd"`
	Model       string `xml:"Model"`
	Serial      string `xml:"Serial"`
	FirmVersion string `xml:"FirmVersion"`
	// SMART attributes (may not be present on all firmware versions)
	SMART struct {
		Attributes []smartAttribute `xml:"attribute"`
	} `xml:"SMART"`
}

type smartAttribute struct {
	ID      string `xml:"id"`
	Name    string `xml:"name"`
	RawValue string `xml:"raw_value"`
}

// volumeManageResp is the response from /cgi-bin/disk/disk_manage.cgi
type volumeManageResp struct {
	XMLName    xml.Name `xml:"QDocRoot"`
	AuthPassed string   `xml:"authPassed"`
	VolumeInfo struct {
		Rows []volumeRow `xml:"row"`
	} `xml:"Volume_Info"`
}

type volumeRow struct {
	VolNo       string `xml:"vol_no"`
	VolLabel    string `xml:"vol_label"`
	CapacityBytes string `xml:"capacity_bytes"`
	FreeBytes   string `xml:"freesize_bytes"`
	UsedPct     string `xml:"used_percent"`
	Filesystem  string `xml:"filesystem_type"`
	RaidLevel   string `xml:"raid_level"`
	MountPath   string `xml:"vol_mount_path"`
	HDList      string `xml:"hd_list"`
}

// --- API -------------------------------------------------------------------

func (c *Client) Login(ctx context.Context) error {
	if c.BaseURL == "" {
		return errors.New("qnap base URL is empty")
	}
	form := url.Values{}
	form.Set("user", c.User)
	form.Set("pwd", base64.StdEncoding.EncodeToString([]byte(c.Password)))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/cgi-bin/authLogin.cgi", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("auth request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var ar authResp
	if err := xml.Unmarshal(body, &ar); err != nil {
		return fmt.Errorf("parse auth xml: %w (body: %s)", err, truncate(body))
	}
	if strings.TrimSpace(ar.AuthSid) == "" || ar.AuthPassed == "0" {
		return errors.New("qnap login rejected (check credentials)")
	}
	c.mu.Lock()
	c.sid = strings.TrimSpace(ar.AuthSid)
	c.mu.Unlock()
	return nil
}

func (c *Client) getSID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sid
}

// FetchAll retrieves system metrics, disk info, and volume details.
func (c *Client) FetchAll(ctx context.Context) (*FetchResult, error) {
	if c.getSID() == "" {
		if err := c.Login(ctx); err != nil {
			return nil, err
		}
	}
	res, err := c.fetchAllOnce(ctx)
	if err == nil {
		return res, nil
	}
	if loginErr := c.Login(ctx); loginErr != nil {
		return nil, fmt.Errorf("fetch failed: %v; re-login also failed: %w", err, loginErr)
	}
	return c.fetchAllOnce(ctx)
}

func (c *Client) fetchAllOnce(ctx context.Context) (*FetchResult, error) {
	sid := c.getSID()

	sysURL := fmt.Sprintf("%s/cgi-bin/management/manaRequest.cgi?subfunc=sysinfo&hd=no&multicpu=1&sid=%s", c.BaseURL, url.QueryEscape(sid))
	diskURL := fmt.Sprintf("%s/cgi-bin/disk/qsmart.cgi?sid=%s", c.BaseURL, url.QueryEscape(sid))

	type result struct {
		body []byte
		err  error
	}
	sysCh := make(chan result, 1)
	diskCh := make(chan result, 1)
	go func() {
		body, err := c.doGet(ctx, sysURL)
		sysCh <- result{body, err}
	}()
	go func() {
		body, err := c.doPost(ctx, diskURL, "func=all_hd_data")
		diskCh <- result{body, err}
	}()

	sysRes := <-sysCh
	diskRes := <-diskCh
	if sysRes.err != nil {
		return nil, fmt.Errorf("sysinfo: %w", sysRes.err)
	}
	if diskRes.err != nil {
		return nil, fmt.Errorf("qsmart: %w", diskRes.err)
	}

	var sr sysinfoResp
	if err := xml.Unmarshal(sysRes.body, &sr); err != nil {
		return nil, fmt.Errorf("parse sysinfo: %w (body: %s)", err, truncate(sysRes.body))
	}
	if sr.AuthPassed == "0" {
		return nil, errors.New("sysinfo auth rejected")
	}

	var dr qsmartResp
	if err := xml.Unmarshal(diskRes.body, &dr); err != nil {
		return nil, fmt.Errorf("parse qsmart: %w (body: %s)", err, truncate(diskRes.body))
	}
	if dr.AuthPassed == "0" {
		return nil, errors.New("qsmart auth rejected")
	}

	snap := &Snapshot{}
	snap.CPUUsage = parsePercent(sr.CPUUsage)
	totalMem := parseFloat(sr.TotalMemory)
	freeMem := parseFloat(sr.FreeMemory)
	if totalMem > 0 {
		snap.MemUsage = (totalMem - freeMem) / totalMem * 100
	}
	snap.SysTempC = parseFloat(sr.SysTempC)
	snap.CPUTempC = parseFloat(sr.CPUTempC)
	snap.FanRPM = parseInt(sr.SystemFan)
	snap.UptimeSeconds = parseInt64(sr.Uptime)
	snap.Model = strings.TrimSpace(sr.Model)
	snap.SerialNumber = strings.TrimSpace(sr.SerialNumber)
	snap.FirmwareVersion = strings.TrimSpace(sr.FirmwareVer)

	disks := make([]DiskInfo, 0, len(dr.DiskInfo.Entries))
	for _, e := range dr.DiskInfo.Entries {
		capStr := strings.TrimSpace(e.Capacity)
		d := DiskInfo{
			HDNo:         strings.TrimSpace(e.HDNo),
			Alias:        strings.TrimSpace(e.DiskAlias),
			Model:        strings.TrimSpace(e.Model),
			Serial:       strings.TrimSpace(e.Serial),
			Firmware:     strings.TrimSpace(e.FirmVersion),
			Vendor:       strings.TrimSpace(e.Vendor),
			Capacity:     capStr,
			CapacityBytes: parseSize(capStr),
			Health:       strings.TrimSpace(e.Health),
			TempC:        parseInt(e.TempC),
			IsSSD:        strings.TrimSpace(e.IsSSD) == "1",
			DiskStatus:   parseInt(e.DiskStatus),
		}
		// Extract SMART attributes if present
		for _, attr := range e.SMART.Attributes {
			switch attr.ID {
			case "9":
				d.PowerOnHours = parseInt64(attr.RawValue)
			case "5":
				d.ReallocatedSectors = parseInt64(attr.RawValue)
			}
		}
		disks = append(disks, d)
	}

	// Now fetch volume details using the volume IDs from disk info.
	volumes, err := c.fetchVolumes(ctx, sid)
	if err != nil {
		log.Printf("qnap: volume fetch failed (non-fatal): %v", err)
	}

	return &FetchResult{
		Snap:        snap,
		Disks:       disks,
		Volumes:     volumes,
		HDTempWarn:  parseInt(dr.HDTempWarn),
		HDTempErr:   parseInt(dr.HDTempErr),
		SSDTempWarn: parseInt(dr.SSDTempWarn),
		SSDTempErr:  parseInt(dr.SSDTempErr),
	}, nil
}

func (c *Client) fetchVolumes(ctx context.Context, sid string) ([]VolumeDetail, error) {
	volURL := fmt.Sprintf("%s/cgi-bin/disk/disk_manage.cgi?sid=%s&store=volumeInfo", c.BaseURL, url.QueryEscape(sid))
	// Request all volumes. The volumeID list can vary; use a wide range.
	form := fmt.Sprintf("volumeID=1%%2C2%%2C3%%2C4%%2C5%%2C6%%2C7%%2C8&func=extra_get&Volume_Info=1&qpkg_list=0&share_info=0&dc=%.16f", rand.Float64())

	body, err := c.doPost(ctx, volURL, form)
	if err != nil {
		return nil, err
	}

	var vr volumeManageResp
	if err := xml.Unmarshal(body, &vr); err != nil {
		return nil, fmt.Errorf("parse disk_manage: %w (body: %s)", err, truncate(body))
	}
	if vr.AuthPassed == "0" {
		return nil, errors.New("disk_manage auth rejected")
	}

	volumes := make([]VolumeDetail, 0, len(vr.VolumeInfo.Rows))
	for _, row := range vr.VolumeInfo.Rows {
		capBytes := parseInt64(row.CapacityBytes)
		freeBytes := parseInt64(row.FreeBytes)
		usedBytes := capBytes - freeBytes
		usedPct := parsePercent(row.UsedPct)
		volumes = append(volumes, VolumeDetail{
			VolNo:         parseInt(row.VolNo),
			Label:         strings.TrimSpace(row.VolLabel),
			CapacityBytes: capBytes,
			UsedBytes:     usedBytes,
			FreeBytes:     freeBytes,
			UsedPct:       usedPct,
			Filesystem:    strings.TrimSpace(row.Filesystem),
			RaidLevel:     parseInt(row.RaidLevel),
			MountPath:     strings.TrimSpace(row.MountPath),
			HDList:        strings.TrimSpace(row.HDList),
		})
	}
	return volumes, nil
}

func (c *Client) doGet(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	return c.doReq(req)
}

func (c *Client) doPost(ctx context.Context, u, body string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	return c.doReq(req)
}

func (c *Client) doReq(req *http.Request) ([]byte, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("status 404 (likely session expired)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// --- helpers ---------------------------------------------------------------

func parsePercent(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	s = strings.TrimSpace(s)
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseFloat(s string) float64 {
	v, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
	return v
}

func parseInt(s string) int {
	v, _ := strconv.Atoi(strings.TrimSpace(s))
	return v
}

func parseInt64(s string) int64 {
	v, _ := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v
}

func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	parts := strings.Fields(s)
	num, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0
	}
	if len(parts) == 1 {
		return int64(num)
	}
	switch strings.ToUpper(parts[1]) {
	case "KB":
		return int64(num * 1024)
	case "MB":
		return int64(num * 1024 * 1024)
	case "GB":
		return int64(num * 1024 * 1024 * 1024)
	case "TB":
		return int64(num * 1024 * 1024 * 1024 * 1024)
	case "PB":
		return int64(num * 1024 * 1024 * 1024 * 1024 * 1024)
	default:
		return int64(num)
	}
}

func truncate(b []byte) string {
	const n = 200
	if len(b) > n {
		return string(b[:n]) + "..."
	}
	return string(b)
}

