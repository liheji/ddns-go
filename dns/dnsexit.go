package dns

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jeessy2/ddns-go/v6/config"
	"github.com/jeessy2/ddns-go/v6/util"
)

const dnsexitEndpoint = "https://api.dnsexit.com"

// DnsExitResponse dnsexit API 响应
type DnsExitResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// DnsExit dnsexit 提供商
type DnsExit struct {
	DNS        config.DNS
	Domains    config.Domains
	lastIpv4   string
	lastIpv6   string
	httpClient *http.Client
}

// Init 初始化
func (d *DnsExit) Init(dnsConf *config.DnsConfig, ipv4cache *util.IpCache, ipv6cache *util.IpCache) {
	d.Domains.Ipv4Cache = ipv4cache
	d.Domains.Ipv6Cache = ipv6cache
	d.lastIpv4 = ipv4cache.Addr
	d.lastIpv6 = ipv6cache.Addr

	d.DNS = dnsConf.DNS
	d.Domains.GetNewIp(dnsConf)
	d.httpClient = dnsConf.GetHTTPClient()
}

// AddUpdateDomainRecords 添加或更新IPv4/IPv6记录
func (d *DnsExit) AddUpdateDomainRecords() config.Domains {
	d.addUpdateDomainRecords("A")
	d.addUpdateDomainRecords("AAAA")
	return d.Domains
}

func (d *DnsExit) addUpdateDomainRecords(recordType string) {
	ipAddr, domains := d.Domains.GetNewIpResult(recordType)

	if ipAddr == "" {
		return
	}

	// 防止多次发送Webhook通知
	if recordType == "A" {
		if d.lastIpv4 == ipAddr {
			util.Log("你的IPv4未变化, 未触发 %s 请求", "DnsExit")
			return
		}
	} else {
		if d.lastIpv6 == ipAddr {
			util.Log("你的IPv6未变化, 未触发 %s 请求", "DnsExit")
			return
		}
	}

	for _, domain := range domains {
		d.updateDomain(domain, ipAddr)
	}
}

// updateDomain 更新单个域名
func (d *DnsExit) updateDomain(domain *config.Domain, ipAddr string) {
	url := fmt.Sprintf("%s/dns/ud/?apikey=%s&host=%s&ip=%s",
		dnsexitEndpoint, d.DNS.Secret, domain.String(), ipAddr)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		util.Log("DnsExit 创建请求失败! 异常信息: %s", err)
		domain.UpdateStatus = config.UpdatedFailed
		return
	}

	resp, err := d.httpClient.Do(req)
	body, err := util.GetHTTPResponseOrg(resp, err)
	if err != nil {
		util.Log("DnsExit 请求失败! 域名: %s, 异常信息: %s", domain, err)
		domain.UpdateStatus = config.UpdatedFailed
		return
	}

	var result DnsExitResponse
	if err := json.Unmarshal(body, &result); err != nil {
		util.Log("DnsExit 解析响应失败! 域名: %s, 响应: %s", domain, string(body))
		domain.UpdateStatus = config.UpdatedFailed
		return
	}

	switch result.Code {
	case 0:
		util.Log("更新域名解析 %s 成功! IP: %s", domain, ipAddr)
		domain.UpdateStatus = config.UpdatedSuccess
	case 1:
		util.Log("你的IP %s 没有变化, 域名 %s", ipAddr, domain)
		domain.UpdateStatus = config.UpdatedNothing
	case 8:
		util.Log("DnsExit 更新太频繁: %s", result.Message)
		domain.UpdateStatus = config.UpdatedFailed
	default:
		util.Log("DnsExit 更新失败! 域名: %s, 响应: %s (code=%d)", domain, result.Message, result.Code)
		domain.UpdateStatus = config.UpdatedFailed
	}
}
