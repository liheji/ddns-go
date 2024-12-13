package dns

import (
	"time"

	"github.com/jeessy2/ddns-go/v6/config"
	"github.com/jeessy2/ddns-go/v6/util"
)

// DNS interface
type DNS interface {
	Init(dnsConf *config.DnsConfig, ipv4cache *util.IpCache, ipv6cache *util.IpCache)
	// 添加或更新IPv4/IPv6记录
	AddUpdateDomainRecords() (domains config.Domains)
}

var (
	Addresses = []string{
		alidnsEndpoint,
		baiduEndpoint,
		zonesAPI,
		recordListAPI,
		huaweicloudEndpoint,
		nameCheapEndpoint,
		nameSiloListRecordEndpoint,
		porkbunEndpoint,
		tencentCloudEndPoint,
		dynadotEndpoint,
	}

	Ipcache = [][2]util.IpCache{}
)

// RunTimer 定时运行
func RunTimer(delay time.Duration) {
	for {
		RunOnce()
		time.Sleep(delay)
	}
}

// RunOnce RunOnce
func RunOnce() {
	conf, err := config.GetConfigCached()
	if err != nil {
		return
	}
	if util.ForceCompareGlobal || len(Ipcache) != len(conf.DnsConf) {
		Ipcache = [][2]util.IpCache{}
		for range conf.DnsConf {
			Ipcache = append(Ipcache, [2]util.IpCache{{}, {}})
		}
	}

	var i = 0
	for _, dc := range conf.DnsConf {
		if dc.Ipv4.GetType == "zerotier" {
			Ipcache = Ipcache[:len(Ipcache)-1]
			data, err := util.GetNetworkMemberList(dc.Ipv4.Zerotier)
			if err != nil {
				continue
			}
			for _, item := range *data {
				if util.TrimDot(item.Name) == "" {
					continue
				}
				newDc := util.DeepCopy(dc)
				newDc.Ipv4.Zerotier = util.TrimComma(newDc.Ipv4.Zerotier) + "," + util.TrimComma(item.NodeID)
				Ipcache = append(Ipcache, [2]util.IpCache{{}, {}})
				dealOneDns(newDc, &conf, i)
				i++
			}
		} else {
			dealOneDns(&dc, &conf, i)
			i++
		}
	}

	util.ForceCompareGlobal = false
}

func dealOneDns(dc *config.DnsConfig, conf *config.Config, i int) {
	var dnsSelected DNS
	switch dc.DNS.Name {
	case "alidns":
		dnsSelected = &Alidns{}
	case "tencentcloud":
		dnsSelected = &TencentCloud{}
	case "trafficroute":
		dnsSelected = &TrafficRoute{}
	case "dnspod":
		dnsSelected = &Dnspod{}
	case "cloudflare":
		dnsSelected = &Cloudflare{}
	case "huaweicloud":
		dnsSelected = &Huaweicloud{}
	case "callback":
		dnsSelected = &Callback{}
	case "baiducloud":
		dnsSelected = &BaiduCloud{}
	case "porkbun":
		dnsSelected = &Porkbun{}
	case "godaddy":
		dnsSelected = &GoDaddyDNS{}
	case "namecheap":
		dnsSelected = &NameCheap{}
	case "namesilo":
		dnsSelected = &NameSilo{}
	case "vercel":
		dnsSelected = &Vercel{}
	case "dynadot":
		dnsSelected = &Dynadot{}
	default:
		dnsSelected = &Alidns{}
	}
	dnsSelected.Init(dc, &Ipcache[i][0], &Ipcache[i][1])
	domains := dnsSelected.AddUpdateDomainRecords()
	// webhook
	v4Status, v6Status := config.ExecWebhook(&domains, conf)
	// 重置单个cache
	if v4Status == config.UpdatedFailed {
		Ipcache[i][0] = util.IpCache{}
	}
	if v6Status == config.UpdatedFailed {
		Ipcache[i][1] = util.IpCache{}
	}
}
