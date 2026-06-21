package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/jeessy2/ddns-go/v6/config"
	"github.com/jeessy2/ddns-go/v6/util"
)

const dnsNekoEndpoint = "https://www.dnsneko.com"

// DnsNekoAPIResponse DnsNeko API 通用响应
type DnsNekoAPIResponse struct {
	Code      int             `json:"code"`
	ErrorCode *string         `json:"errorCode"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data"`
}

// DnsNekoDomain 域名列表中的域名
type DnsNekoDomain struct {
	ID          string `json:"id"`
	Domain      string `json:"domain"`
	Status      int    `json:"status"`
	Expired     bool   `json:"expired"`
	ExpireTime  string `json:"expireTime"`
	RecordCount string `json:"recordCount"`
}

// DnsNekoDomainList 域名列表响应 data
type DnsNekoDomainList struct {
	Domains []DnsNekoDomain `json:"domains"`
	Total   string          `json:"total"`
	Size    string          `json:"size"`
	Current string          `json:"current"`
	Pages   string          `json:"pages"`
}

// DnsNekoRecord DNS 记录
type DnsNekoRecord struct {
	ID         string `json:"id"`
	DomainID   any    `json:"domainId"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Value      string `json:"value"`
	Line       string `json:"line"`
	TTL        int    `json:"ttl"`
	Priority   any    `json:"priority"`
	Remark     string `json:"remark"`
	Status     int    `json:"status"`
	UpdateTime any    `json:"updateTime"`
}

// DnsNekoRecordList 记录列表响应 data
type DnsNekoRecordList struct {
	DomainID string          `json:"domainId"`
	Domain   string          `json:"domain"`
	Records  []DnsNekoRecord `json:"records"`
	Total    string          `json:"total"`
	Size     string          `json:"size"`
	Current  string          `json:"current"`
	Pages    string          `json:"pages"`
}

// DnsNekoRecordBody 创建/更新记录的请求体
type DnsNekoRecordBody struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Value  string `json:"value"`
	Line   string `json:"line"`
	TTL    int    `json:"ttl"`
	Remark string `json:"remark,omitempty"`
}

// DnsNeko DnsNeko 提供商
type DnsNeko struct {
	DNS        config.DNS
	Domains    config.Domains
	TTL        string
	lastIpv4   string
	lastIpv6   string
	httpClient *http.Client
	domainMap  map[string]string // 域名 → 域名ID 缓存
}

// Init 初始化
func (d *DnsNeko) Init(dnsConf *config.DnsConfig, ipv4cache *util.IpCache, ipv6cache *util.IpCache) {
	d.Domains.Ipv4Cache = ipv4cache
	d.Domains.Ipv6Cache = ipv6cache
	d.lastIpv4 = ipv4cache.Addr
	d.lastIpv6 = ipv6cache.Addr

	d.DNS = dnsConf.DNS
	d.Domains.GetNewIp(dnsConf)
	if dnsConf.TTL == "" {
		d.TTL = "600"
	} else {
		d.TTL = dnsConf.TTL
	}
	d.httpClient = dnsConf.GetHTTPClient()

	// 一次性获取域名列表，缓存域名→ID映射
	d.domainMap = d.buildDomainMap()
}

// buildDomainMap 构建域名→域名ID映射（只查第一页，page=1, size=100）
func (d *DnsNeko) buildDomainMap() map[string]string {
	domainMap := make(map[string]string)

	apiResp, err := d.request(http.MethodGet,
		"/api/v1/dns/domains?page=1&size=100", nil)
	if err != nil {
		util.Log("DnsNeko获取域名列表失败: %s", err)
		return domainMap
	}

	var domainList DnsNekoDomainList
	if err := json.Unmarshal(apiResp.Data, &domainList); err != nil {
		util.Log("DnsNeko解析域名列表失败: %s", err)
		return domainMap
	}

	for _, domain := range domainList.Domains {
		domainMap[domain.Domain] = domain.ID
	}

	util.Log("DnsNeko已加载 %d 个域名", len(domainMap))
	return domainMap
}

// AddUpdateDomainRecords 添加或更新IPv4/IPv6记录
func (d *DnsNeko) AddUpdateDomainRecords() config.Domains {
	d.addUpdateDomainRecords("A")
	d.addUpdateDomainRecords("AAAA")
	return d.Domains
}

func (d *DnsNeko) addUpdateDomainRecords(recordType string) {
	ipAddr, domains := d.Domains.GetNewIpResult(recordType)

	if ipAddr == "" {
		return
	}

	// 防止多次发送Webhook通知
	if recordType == "A" {
		if d.lastIpv4 == ipAddr {
			util.Log("你的IPv4未变化, 未触发 %s 请求", "DnsNeko")
			return
		}
	} else {
		if d.lastIpv6 == ipAddr {
			util.Log("你的IPv6未变化, 未触发 %s 请求", "DnsNeko")
			return
		}
	}

	for _, domain := range domains {
		err := d.updateOrCreateRecord(domain, ipAddr, recordType)
		if err != nil {
			util.Log("更新域名解析 %s 失败! 异常信息: %s", domain, err)
			domain.UpdateStatus = config.UpdatedFailed
		} else {
			util.Log("更新域名解析 %s 成功! IP: %s", domain, ipAddr)
			domain.UpdateStatus = config.UpdatedSuccess
		}
	}
}

// findDomain 从 domainMap 中查找匹配的域名ID，并返回正确的子域名
// 通过后缀匹配支持多级域名，如用户注册了 sub.example.com，配置 test.sub.example.com 时
// domainMap 中有 "sub.example.com"，则匹配为 domainID + 子域名 "test"
func (d *DnsNeko) findDomain(fullDomain string) (domainID, subDomain string, err error) {
	// 先尝试精确匹配
	if id, ok := d.domainMap[fullDomain]; ok {
		return id, "@", nil
	}

	// 逐级剥离前缀，后缀匹配
	parts := strings.Split(fullDomain, ".")
	for i := 1; i <= len(parts)-2; i++ {
		candidate := strings.Join(parts[i:], ".")
		if id, ok := d.domainMap[candidate]; ok {
			subDomain := strings.Join(parts[:i], ".")
			return id, subDomain, nil
		}
	}

	return "", "", fmt.Errorf("域名 %s 在 DnsNeko 账号中未找到", fullDomain)
}

// updateOrCreateRecord 更新或创建 DNS 记录
func (d *DnsNeko) updateOrCreateRecord(domain *config.Domain, ipAddr string, recordType string) error {
	// 后缀匹配查找域名ID（支持多级域名）
	domainID, subDomain, err := d.findDomain(domain.String())
	if err != nil {
		return err
	}

	// 查询已有的DNS记录
	record, err := d.getRecord(domainID, subDomain, recordType)
	if err != nil {
		return fmt.Errorf("查询记录失败: %w", err)
	}

	ttl, _ := strconv.Atoi(d.TTL)
	if ttl == 0 {
		ttl = 600
	}

	body := DnsNekoRecordBody{
		Name:  subDomain,
		Type:  recordType,
		Value: ipAddr,
		Line:  "default",
		TTL:   ttl,
	}

	if record != nil {
		// 更新已有记录
		return d.updateRecord(domainID, record.ID, &body)
	}
	// 创建新记录
	return d.createRecord(domainID, &body)
}

// getRecord 查询指定子域名和类型的DNS记录
func (d *DnsNeko) getRecord(domainID, subDomain, recordType string) (*DnsNekoRecord, error) {
	path := fmt.Sprintf("/api/v1/dns/records?domainId=%s&page=1&size=100", domainID)

	apiResp, err := d.request(http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	var recordList DnsNekoRecordList
	if err := json.Unmarshal(apiResp.Data, &recordList); err != nil {
		return nil, fmt.Errorf("解析记录列表失败: %w", err)
	}

	for _, r := range recordList.Records {
		if r.Name == subDomain && r.Type == recordType {
			return &r, nil
		}
	}

	return nil, nil
}

// updateRecord 更新已有记录
func (d *DnsNeko) updateRecord(domainID, recordID string, body *DnsNekoRecordBody) error {
	path := fmt.Sprintf("/api/v1/dns/records/%s/%s", domainID, recordID)

	_, err := d.request(http.MethodPut, path, body)
	return err
}

// createRecord 创建新记录
func (d *DnsNeko) createRecord(domainID string, body *DnsNekoRecordBody) error {
	path := fmt.Sprintf("/api/v1/dns/records/%s", domainID)

	_, err := d.request(http.MethodPost, path, body)
	return err
}

// request 发送 HTTP 请求，返回 API 通用响应
func (d *DnsNeko) request(method, path string, body any) (*DnsNekoAPIResponse, error) {
	url := dnsNekoEndpoint + path

	var bodyReader *bytes.Buffer
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("序列化请求体失败: %w", err)
		}
		bodyReader = bytes.NewBuffer(jsonBody)
	} else {
		bodyReader = bytes.NewBuffer(nil)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, err
	}

	// 设置认证头
	req.Header.Set("X-DNSNEKO-USERNAME", strings.TrimSpace(d.DNS.ID))
	req.Header.Set("X-DNSNEKO-API-KEY", strings.TrimSpace(d.DNS.Secret))
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	bodyBytes, err := util.GetHTTPResponseOrg(resp, err)
	if err != nil {
		return nil, err
	}

	var apiResp DnsNekoAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("解析API响应失败: %w", err)
	}

	if apiResp.Code != 200 {
		return nil, fmt.Errorf("API返回错误 (code=%d): %s", apiResp.Code, apiResp.Message)
	}

	return &apiResp, nil
}
