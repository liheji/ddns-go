package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/jeessy2/ddns-go/v6/config"
	"github.com/jeessy2/ddns-go/v6/util"
)

const digitaloceanEndpoint = "https://api.digitalocean.com"

// DigitalOcean DigitalOcean DNS provider
type DigitalOcean struct {
	DNS        config.DNS
	Domains    config.Domains
	TTL        int
	httpClient *http.Client
}

// DigitalOceanRecordsResp list records response
type DigitalOceanRecordsResp struct {
	DomainRecords []DigitalOceanRecord `json:"domain_records"`
}

// DigitalOceanRecord DNS record entity
type DigitalOceanRecord struct {
	ID   int    `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
	Data string `json:"data"`
	TTL  int    `json:"ttl"`
}

// Init init
func (do *DigitalOcean) Init(dnsConf *config.DnsConfig, ipv4cache *util.IpCache, ipv6cache *util.IpCache) {
	do.Domains.Ipv4Cache = ipv4cache
	do.Domains.Ipv6Cache = ipv6cache
	do.DNS = dnsConf.DNS
	do.Domains.GetNewIp(dnsConf)
	if dnsConf.TTL == "" {
		// default 600
		do.TTL = 600
	} else {
		ttl, err := strconv.Atoi(dnsConf.TTL)
		if err != nil {
			do.TTL = 600
		} else {
			do.TTL = ttl
		}
	}
	do.httpClient = dnsConf.GetHTTPClient()
}

// AddUpdateDomainRecords add or update IPv4/IPv6 records
func (do *DigitalOcean) AddUpdateDomainRecords() config.Domains {
	do.addUpdateDomainRecords("A")
	do.addUpdateDomainRecords("AAAA")
	return do.Domains
}

func (do *DigitalOcean) addUpdateDomainRecords(recordType string) {
	ipAddr, domains := do.Domains.GetNewIpResult(recordType)

	if ipAddr == "" {
		return
	}

	for _, domain := range domains {
		// Query existing records
		params := url.Values{}
		params.Set("type", recordType)
		params.Set("name", domain.String())

		var records DigitalOceanRecordsResp
		err := do.request(
			"GET",
			fmt.Sprintf(digitaloceanEndpoint+"/v2/domains/%s/records?%s", domain.DomainName, params.Encode()),
			nil,
			&records,
		)

		if err != nil {
			util.Log("查询域名记录异常! %s", err)
			domain.UpdateStatus = config.UpdatedFailed
			return
		}

		if len(records.DomainRecords) > 0 {
			// Update
			do.modify(records, domain, ipAddr)
		} else {
			// Create
			do.create(domain, recordType, ipAddr)
		}
	}
}

// create create a new DNS record
func (do *DigitalOcean) create(domain *config.Domain, recordType string, ipAddr string) {
	record := DigitalOceanRecord{
		Type: recordType,
		Name: domain.GetSubDomain(),
		Data: ipAddr,
		TTL:  do.TTL,
	}

	var result DigitalOceanRecordsResp
	err := do.request(
		"POST",
		fmt.Sprintf(digitaloceanEndpoint+"/v2/domains/%s/records", domain.DomainName),
		record,
		&result,
	)

	if err != nil {
		util.Log("新增域名解析 %s 失败! 异常信息: %s", domain, err)
		domain.UpdateStatus = config.UpdatedFailed
		return
	}

	util.Log("新增域名解析 %s 成功! IP: %s", domain, ipAddr)
	domain.UpdateStatus = config.UpdatedSuccess
}

// modify update existing DNS records
func (do *DigitalOcean) modify(result DigitalOceanRecordsResp, domain *config.Domain, ipAddr string) {
	for _, record := range result.DomainRecords {
		// Same IP, skip
		if record.Data == ipAddr {
			util.Log("你的IP %s 没有变化, 域名 %s", ipAddr, domain)
			continue
		}

		// PATCH - only update changed fields
		updateData := map[string]any{
			"data": ipAddr,
		}

		var resp DigitalOceanRecordsResp
		err := do.request(
			"PATCH",
			fmt.Sprintf(digitaloceanEndpoint+"/v2/domains/%s/records/%d", domain.DomainName, record.ID),
			updateData,
			&resp,
		)

		if err != nil {
			util.Log("更新域名解析 %s 失败! 异常信息: %s", domain, err)
			domain.UpdateStatus = config.UpdatedFailed
			return
		}

		util.Log("更新域名解析 %s 成功! IP: %s", domain, ipAddr)
		domain.UpdateStatus = config.UpdatedSuccess
	}
}

// request unified HTTP request method
func (do *DigitalOcean) request(method string, urlStr string, data any, result any) (err error) {
	jsonStr := make([]byte, 0)
	if data != nil {
		jsonStr, _ = json.Marshal(data)
	}
	req, err := http.NewRequest(
		method,
		urlStr,
		bytes.NewBuffer(jsonStr),
	)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", "Bearer "+do.DNS.Secret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := do.httpClient.Do(req)
	err = util.GetHTTPResponse(resp, err, result)

	return
}
