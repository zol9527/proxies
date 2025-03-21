// *
// * Author       :loyd
// * Date         :2024-11-10 21:29:46
// * LastEditors  :loyd
// * LastEditTime :2024-11-11 14:56:16
// * Description  :scrape ip from web
// *

package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"zol9527/proxies/pkg/check"
	"zol9527/proxies/pkg/logger"
	"zol9527/proxies/pkg/resource"

	"github.com/PuerkitoBio/goquery"
	"github.com/duke-git/lancet/v2/convertor"
	"github.com/duke-git/lancet/v2/fileutil"
	"github.com/duke-git/lancet/v2/netutil"
	"github.com/duke-git/lancet/v2/slice"
	"github.com/duke-git/lancet/v2/strutil"
	"github.com/duke-git/lancet/v2/validator"
)

// LoadConfiguration 📂 加载并返回配置信息 LoadConfiguration 📂 加载并返回配置信息
//
// 该函数执行以下操作：
// 1. 获取默认配置文件路径
// 2. 从配置文件中读取配置信息 2. 从配置文件中读取配置信息
//
// 返回值:
//   - *resource.Config: 包含所有配置信息的配置对象   - *resource.Config: 包含所有配置信息的配置对象
//
// 注意：如果在获取配置文件路径或加载配置过程中发生错误，函数会触发panic
func LoadConfiguration() *resource.Config {
	// 加载配置
	path, err := resource.DefaultConfigPath()
	if err != nil {
		panic(err)
	}
	// 读取配置
	config, loadErr := resource.LoadConfig(path)
	if loadErr != nil {
		panic(loadErr)
	}

	return config
}

// RequestPage 🌐 从指定的配置中获取网页内容并解析出IP地址 RequestPage 🌐 从指定的配置中获取网页内容并解析出IP地址
//
// 该函数接收一个资源配置指针作为参数，遍历配置中的所有平台和URL，L，
// 向每个URL发送HTTP请求，然后解析响应内容以提取IP地址。 向每个URL发送HTTP请求，然后解析响应内容以提取IP地址。
//
// 参数:
//   - config: 资源配置指针，包含平台、URL等信息   - config: 资源配置指针，包含平台、URL等信息
//
// 返回值:
//   - []string: 从所有URL中提取的IP地址列表   - []string: 从所有URL中提取的IP地址列表
//
// 注意:
//   - 如果请求失败或解析不到IP地址，将记录错误或警告日志，并继续处理下一个URL
func RequestPage(config *resource.Config) []string {
	logger := logger.GetLogger()
	var ips []string

	// 遍历全部的平台, 每个平台可能有多个 URL
	for _, platform := range config.Platforms {
		for _, url := range platform.URLs {
			// 发送 HTTP 请求
			request := &netutil.HttpRequest{
				RawURL: url,
				Method: platform.Method,
			}
			client := netutil.NewHttpClient()
			resp, err := client.SendRequest(request)
			if err != nil {
				// 处理错误
				logger.Error(fmt.Sprintf("❌ 请求失败 [%s]: %v", url, err))
				continue
			}

			// 转化 http response 为字符串串
			defer resp.Body.Close()
			byteContent, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				// 处理读取错误
				logger.Error(fmt.Sprintf("❌ 读取响应内容失败 [%s]: %v", url, readErr))
				continue
			}
			strContent := string(byteContent)

			// 解析 HTML 中的 URL
			extractedIPs := ParseURLs(strContent)
			if len(extractedIPs) == 0 {
				logger.Warn(fmt.Sprintf("⚠️ 未在URL中找到可用IP地址: %s", url))
				continue
			}

			// 处理解析到的 IP
			ips = append(ips, extractedIPs...)
			logger.Info(fmt.Sprintf("✅ 从 %s 成功解析到 %d 个IP地址", url, len(extractedIPs)))
		}
	}

	logger.Info(fmt.Sprintf("🔍 总共收集到 %d 个IP地址", len(ips)))
	return ips
}

// Scrape 🚀 爬取、测试和输出代理IP的主函数 Scrape 🚀 爬取、测试和输出代理IP的主函数
//
// 该函数执行完整的代理收集流程:理收集流程:
// 1. 加载配置信息
// 2. 爬取各来源的IP
// 3. 加载之前缓存的IPIP
// 4. 对IP进行去重
// 5. 测试代理的可用性
// 6. 输出有效代理到文件
func Scrape() {
	logger := logger.GetLogger()

	// 加载配置并请求页面
	config := LoadConfiguration()
	ips := RequestPage(config)

	// 加载之前保存的IP
	previousIPs := LoadPreviousIPs()
	if len(previousIPs) > 0 {
		logger.Info(fmt.Sprintf("📂 加载到 %d 个历史IP记录", len(previousIPs)))
		ips = append(ips, previousIPs...)
	}

	// 使用lancet的slice.Unique进行去重
	uniqueIPsSlice := slice.Unique(ips)

	logger.Info(fmt.Sprintf("🧹 去重后待测试IP: %d 个", len(uniqueIPsSlice)))

	// 使用多线程测试 IP 可用性
	validIPs := TestProxy(uniqueIPsSlice)
	logger.Info(fmt.Sprintf("✨ 测试后有效IP: %d 个", len(validIPs)))

	// 输出写入文件
	Output(validIPs)
}

// LoadPreviousIPs 📋 加载之前收集的IP列表 LoadPreviousIPs 📋 加载之前收集的IP列表
//
// 该函数从ip.txt文件中读取之前保存的IP列表 该函数从ip.txt文件中读取之前保存的IP列表
//
// 返回值:
//   - []string: 之前保存的IP地址列表
func LoadPreviousIPs() []string {
	logger := logger.GetLogger()
	// 定义多个可能的文件路径
	possiblePaths := []string{
		"ip.txt",       // 当前目录
		"../ip.txt",    // 父级目录
		"../../ip.txt", // 父级的父级目录级目录
		"/tmp/ip.txt",  // 临时目录
	}

	// 🔍 查找第一个存在的文件路径文件路径
	filePath := ""
	for _, path := range possiblePaths {
		if fileutil.IsExist(path) {
			filePath = path
			logger.Info(fmt.Sprintf("📁 找到历史IP文件: %s", filePath))
			break
		}
	}

	// 如果没有找到文件，使用默认路径
	if filePath == "" {
		filePath = "ip.txt" // 默认使用当前目录
		logger.Debug("📝 未找到现有IP文件，将使用默认路径: ip.txt")
	}
	var previousIPs []string

	// 检查文件是否存在
	if !fileutil.IsExist(filePath) {
		logger.Info("🔍 未找到历史IP文件，将仅使用新抓取的IP")
		return previousIPs
	}

	// 读取文件内容
	content, err := fileutil.ReadFileToString(filePath)
	if err != nil {
		logger.Warn(fmt.Sprintf("⚠️ 读取历史IP文件失败: %v", err))
		return previousIPs
	}

	// 解析文件内容
	lines := strutil.SplitAndTrim(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 尝试处理行内容，支持多种格式的IP地址
		if strings.Contains(line, "{") && strings.Contains(line, "}") {
			// 可能是JSON格式，处理可能的转义
			processedLine := line

			// 处理外层可能有引号的JSON字符串
			if strings.HasPrefix(line, "\"") && strings.HasSuffix(line, "\"") {
				// 去除外层引号，并处理转义字符转义字符
				var err error
				processedLine, err = strconv.Unquote(line)
				if err != nil {
					// 如果无法正确解除引号，则使用原始字符串符串
					processedLine = line
				}
			}

			// 尝试作为JSON解析
			var ipInfo map[string]interface{}
			if err := json.Unmarshal([]byte(processedLine), &ipInfo); err == nil {
				// 成功解析为JSON
				if ipVal, ok := ipInfo["ip"].(string); ok {
					if strings.Contains(ipVal, ":") {
						// IP已包含端口
						previousIPs = append(previousIPs, ipVal)
					} else if portVal, ok := ipInfo["port"]; ok {
						// 合并IP和端口
						portStr := fmt.Sprintf("%v", portVal)
						previousIPs = append(previousIPs, fmt.Sprintf("%s:%s", ipVal, portStr))
					}
					continue
				}
			}

			// JSON解析失败，尝试使用正则表达式提取
			ipRegex := regexp.MustCompile(`"ip"\s*:\s*"((?:\d{1,3}\.){3}\d{1,3}(?::\d{1,5})?)"`)
			matches := ipRegex.FindStringSubmatch(processedLine)
			if len(matches) > 1 {
				ipValue := matches[1]
				if !strings.Contains(ipValue, ":") {
					// 尝试找端口
					portRegex := regexp.MustCompile(`"port"\s*:\s*"?(\d{1,5})"?`)
					portMatches := portRegex.FindStringSubmatch(processedLine)
					if len(portMatches) > 1 {
						ipValue = fmt.Sprintf("%s:%s", ipValue, portMatches[1])
					}
				}
				previousIPs = append(previousIPs, ipValue)
			}
		} else {
			// 检查是否为直接的IP:PORT格式
			ipPortRegex := regexp.MustCompile(`^((?:\d{1,3}\.){3}\d{1,3}:\d{1,5})$`)
			if ipPortRegex.MatchString(line) {
				previousIPs = append(previousIPs, line)
			}
		}
	}

	logger.Debug(fmt.Sprintf("🔄 成功从历史文件中解析 %d 个IP", len(previousIPs)))
	return previousIPs
}

// TestProxy 🔍 测试代理IP的可用性和类型
//
// 该函数接收IP地址列表，并通过多线程并发测试每个IP的:
// - HTTP代理功能
// - HTTPS代理功能
// - SOCKS5代理功能
// - 匿名性级别
//
// 参数:
//   - ips: 需要测试的IP地址列表
//
// 返回值:
//   - []string: 有效IP地址的JSON格式字符串列表
func TestProxy(ips []string) []string {
	logger := logger.GetLogger()

	// 根据CPU核心数和待测试IP数量动态调整线程数
	cpuCount := runtime.NumCPU()
	// 线程数 = min(CPU核心数 * 8, IP数量, 200)
	threadNum := cpuCount * 4
	if threadNum > len(ips) {
		threadNum = len(ips)
	}
	// 设置线程上限，避免资源耗尽
	if threadNum > 80 {
		threadNum = 80
	}

	logger.Info(fmt.Sprintf("🚀 使用 %d 个线程进行代理测试 (CPU核心: %d)", threadNum, cpuCount))

	var wg sync.WaitGroup

	// 定义IP信息结构体
	type IpInfo struct {
		IP        string `json:"ip"`
		Http      bool   `json:"http"`
		Https     bool   `json:"https"`
		Socks5    bool   `json:"socks5"`
		Anonymity string `json:"anonymity"`
	}

	// 使用并发安全的Map存储结果
	var mutex sync.Mutex
	IpList := make([]IpInfo, 0, len(ips)/3) // 预分配容量，假设约1/3的IP可用

	// 使用并发安全的计数器追踪统计信息
	var statsMutex sync.Mutex
	validCount := 0
	httpCount := 0
	httpsCount := 0
	socks5Count := 0

	// 使用原子计数器跟踪进度
	var processedCount int32
	totalCount := len(ips)

	// 对IP进行分批处理，避免一次性创建过多goroutine
	const batchSize = 1000
	batchCount := (totalCount + batchSize - 1) / batchSize // 向上取整计算批次数

	logger.Info(fmt.Sprintf("📊 IP测试将分 %d 批进行，每批最多 %d 个", batchCount, batchSize))

	// 定义检查函数 - 对每个IP进行功能性测试
	checkFunc := func(ip string) {
		defer wg.Done()

		// 更新处理计数并定期输出进度
		count := atomic.AddInt32(&processedCount, 1)
		progress := float64(count) / float64(totalCount) * 100
		// 减少进度日志频率，只在整数百分比变化时显示
		if count == 1 || count%10 == 0 || count == int32(totalCount) {
			logger.Info(fmt.Sprintf("🔄 代理测试进度: %d/%d (%.1f%%)", count, totalCount, progress))
		}

		// 快速检查 HTTP 代理功能，这是基本可用性检查
		isHttp := check.FastCheckHttp(ip)
		if !isHttp {
			return
		}

		// HTTP检查通过，更新计数
		statsMutex.Lock()
		httpCount++
		statsMutex.Unlock()

		// 并行检查其他功能
		var isHttps, isSocket5 bool
		var anonymity string
		var wg2 sync.WaitGroup
		wg2.Add(3)

		// 检查HTTPS
		go func() {
			defer wg2.Done()
			isHttps = check.CheckHttpsResponse(ip, "", "")
			if isHttps {
				statsMutex.Lock()
				httpsCount++
				statsMutex.Unlock()
			}
		}()

		// 检查SOCKS5
		go func() {
			defer wg2.Done()
			isSocket5 = check.CheckSocket5Response(ip)
			if isSocket5 {
				statsMutex.Lock()
				socks5Count++
				statsMutex.Unlock()
			}
		}()

		// 检查匿名性
		go func() {
			defer wg2.Done()
			anonymity = check.CheckProxyAnonymity(ip)
		}()

		// 等待所有检查完成
		wg2.Wait()

		// 组装结果
		ipInfo := IpInfo{
			IP:        ip,
			Http:      isHttp,
			Https:     isHttps,
			Socks5:    isSocket5,
			Anonymity: anonymity,
		}

		// 使用互斥锁安全地添加到结果列表
		mutex.Lock()
		IpList = append(IpList, ipInfo)
		validCount++
		mutex.Unlock()
	}

	// 按批次处理IP
	for batchIndex := 0; batchIndex < batchCount; batchIndex++ {
		start := batchIndex * batchSize
		end := start + batchSize
		if end > totalCount {
			end = totalCount
		}

		batchIps := ips[start:end]
		logger.Info(fmt.Sprintf("🔄 开始处理第 %d/%d 批 (IP数量: %d)", batchIndex+1, batchCount, len(batchIps)))

		// 创建批次的信号量来限制并发
		sem := make(chan struct{}, threadNum)

		// 为这一批次的每个IP启动一个goroutine
		for _, ip := range batchIps {
			wg.Add(1)

			// 使用信号量控制并发
			sem <- struct{}{}

			go func(ipAddr string) {
				defer func() { <-sem }() // 释放信号量
				checkFunc(ipAddr)
			}(ip)
		}

		// 等待当前批次处理完成
		wg.Wait()
		logger.Info(fmt.Sprintf("✅ 第 %d/%d 批处理完成", batchIndex+1, batchCount))
	}

	// 使用更友好的完成信息
	if validCount > 0 {
		logger.Info(fmt.Sprintf("✅ 代理测试完成: 共测试 %d 个IP, 有效IP %d 个 (成功率: %.1f%%)",
			totalCount, validCount, float64(validCount)/float64(totalCount)*100))
		logger.Info(fmt.Sprintf("📊 代理类型统计: HTTP: %d, HTTPS: %d, SOCKS5: %d",
			httpCount, httpsCount, socks5Count))
	} else {
		logger.Warn(fmt.Sprintf("⚠️ 代理测试完成: 共测试 %d 个IP, 未找到有效代理", totalCount))
	}

	// 转化数据类型，最终以字符串的切片传出
	var validIPs []string

	// 使用 lancet 的批量转换
	for _, ipInfo := range IpList {
		jsonStr, err := convertor.ToJson(ipInfo)
		if err == nil {
			validIPs = append(validIPs, jsonStr)
		}
	}

	// 仅在总体转换有问题时记录一次错误
	if len(validIPs) < len(IpList) {
		logger.Warn(fmt.Sprintf("⚠️ 部分IP信息(%d/%d)转换失败", len(IpList)-len(validIPs), len(IpList)))
	}

	logger.Info(fmt.Sprintf("📋 最终获得 %d 个可用代理，准备输出", len(validIPs)))
	return validIPs
}

// ParseURLs 🔍 从HTML内容中提取IP地址和端口组合
//
// 该函数使用三种提取策略:
// 1. 正则表达式匹配整个HTML中的IPv4:端口组合
// 2. 解析HTML表格结构，从表格单元格中提取IP和端口
// 3. 从JSON格式数据中提取主机和端口信息
//
// 参数：
//   - html: 包含要解析的HTML内容的字符串
//
// 返回值：
//   - []string: 格式为"IP:PORT"的字符串切片
func ParseURLs(html string) []string {
	logger := logger.GetLogger()
	var ips []string

	// 策略1: 简单匹配整个HTML中的IPv4地址和端口号
	pattern := `((?:\d{1,3}\.){3}\d{1,3}:\d{1,5})`
	matchedGroups := strutil.RegexMatchAllGroups(pattern, html)
	for _, ipSlice := range matchedGroups {
		ips = append(ips, ipSlice...)
	}

	// 策略2: 从HTML表格结构中提取IP和端口
	if len(ips) == 0 {
		logger.Info("🔍 从表格结构中解析IP和端口...")
		ips = parseFromTable(html)
	}

	// 策略3: 从JSON格式数据中提取
	if len(ips) == 0 {
		logger.Info("🔍 尝试从JSON格式中解析IP和端口...")
		ips = parseFromJson(html)
	}
	return ips
}

// parseFromTable 📊 从HTML表格中提取IP和端口信息
//
// 参数:
//   - html: HTML内容
//
// 返回值:
//   - []string: 格式为"IP:PORT"的字符串切片
func parseFromTable(html string) []string {
	logger := logger.GetLogger()
	var ips []string

	// 使用goquery解析HTML文档
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		logger.Error("❌ 解析HTML文档失败: " + err.Error())
		return ips
	}

	// 查找所有表格行
	doc.Find("table tr").Each(func(index int, row *goquery.Selection) {
		var ip, port string

		// 遍历行中的所有单元格
		row.Find("td").Each(func(i int, cell *goquery.Selection) {
			cellText := strings.TrimSpace(cell.Text())

			// 精确匹配IPv4地址 (确保每个八位字节在0-255范围内)
			if ip == "" {
				ipPattern := `\b(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3}\b`
				ipMatches := strutil.RegexMatchAllGroups(ipPattern, cellText)
				if len(ipMatches) > 0 && len(ipMatches[0]) > 0 {
					candidateIP := ipMatches[0][0]
					// 使用lancet验证IP格式
					if validator.IsIpV4(candidateIP) {
						ip = candidateIP
					}
				}
			}

			// 匹配端口号 (从文本中提取并验证范围)
			if port == "" {
				// 尝试提取端口号，不管是独立的还是嵌入在文本中的
				portMatches := strutil.RegexMatchAllGroups(`\b([0-9]{1,5})\b`, cellText)
				for _, portMatch := range portMatches {
					if len(portMatch) > 0 {
						candidatePort := portMatch[0]
						portNum, err := convertor.ToInt(candidatePort)
						// 验证端口范围在1-65535之间
						if err == nil && portNum > 0 && portNum <= 65535 {
							port = candidatePort
							break
						}
					}
				}
			}
		})

		// 如果在同一行找到了有效的IP和端口
		if ip != "" && port != "" {
			ipPort := fmt.Sprintf("%s:%s", ip, port)
			logger.Debug(fmt.Sprintf("✅ 从表格解析到代理: %s", ipPort))
			ips = append(ips, ipPort)
		}
	})

	logger.Info(fmt.Sprintf("🔢 从表格中总共提取到 %d 个IP地址", len(ips)))
	return ips
}

// parseFromJson 📊 从JSON格式内容中提取IP和端口信息
//
// 参数:
//   - html: 包含JSON数据的HTML内容
//
// 返回值:
//   - []string: 格式为"IP:PORT"的字符串切片
func parseFromJson(html string) []string {
	logger := logger.GetLogger()
	var ips []string

	// 预处理HTML内容 - 处理可能存在的转义字符
	processedHtml := strings.ReplaceAll(html, "\\\"", "\"")

	// 尝试两种匹配模式：标准格式和转义格式
	patterns := []string{
		`"host"\s*:\s*"((?:\d{1,3}\.){3}\d{1,3})".*?"port"\s*:\s*(\d{1,5})`,
		`host\s*:\s*"?((?:\d{1,3}\.){3}\d{1,3})"?.*?port\s*:\s*(\d{1,5})`,
	}

	var jsonMatches [][]string
	for _, pattern := range patterns {
		matches := strutil.RegexMatchAllGroups(pattern, processedHtml)
		jsonMatches = append(jsonMatches, matches...)
	}

	// 如果整体匹配没有结果，尝试逐行解析
	if len(jsonMatches) == 0 {
		logger.Debug("🔄 尝试按行解析JSON数据...")
		lines := strings.Split(processedHtml, "\n")

		for _, line := range lines {
			if strings.Contains(line, "host") && strings.Contains(line, "port") {
				for _, pattern := range patterns {
					lineMatches := strutil.RegexMatchAllGroups(pattern, line)
					jsonMatches = append(jsonMatches, lineMatches...)
				}
			}
		}
	}

	// 处理所有匹配结果
	for _, match := range jsonMatches {
		if len(match) > 2 {
			ip := match[1]
			port := match[2]

			// 验证IP和端口的有效性
			if validator.IsIpV4(ip) {
				portNum, err := convertor.ToInt(port)
				if err == nil && portNum > 0 && portNum <= 65535 {
					ipPort := fmt.Sprintf("%s:%s", ip, port)
					ips = append(ips, ipPort)
					logger.Debug(fmt.Sprintf("✅ 从JSON解析到代理: %s", ipPort))
				}
			}
		}
	}

	logger.Info(fmt.Sprintf("🔢 从JSON格式中总共提取到 %d 个IP地址", len(ips)))
	return ips
}

// Output 💾 将有效IP写入文件
//
// 参数:
//   - ips: 要写入文件的IP地址列表
func Output(ips []string) {
	filepath := "ip.txt"
	logger := logger.GetLogger()

	// 创建或清空文件
	if !fileutil.CreateFile(filepath) {
		panic("❌ 创建文件失败")
	}
	fileutil.ClearFile(filepath)

	// 写入文件内容
	for _, row := range ips {
		fileutil.WriteStringToFile(filepath, row+"\n", true)
	}

	logger.Info(fmt.Sprintf("💾 成功写入 %d 个IP到文件 %s", len(ips), filepath))
}
