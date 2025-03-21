// Author       :loyd
// Date         :2025-03-08 00:40:59
// LastEditors  :loyd
// LastEditTime :2025-03-08 00:52:27
// Description  :代理IP检查工具包
//

package check

import (
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"zol9527/proxies/pkg/logger"

	"github.com/duke-git/lancet/v2/netutil"
)

// FastCheckHttp 🚀 快速验证HTTP代理基本可用性
//
// 这是一个轻量级验证函数，仅进行基本的连接性测试，超时更短
// 用于快速筛选可能有效的代理，减少后续详细测试的数量
//
// 参数:
//   - ip: 代理服务器地址(格式："ip:port")
//
// 返回值:
//   - bool: 代理是否能基本连通
func FastCheckHttp(ip string) bool {
	// 配置极短的超时时间
	timeout := 3 * time.Second

	// 快速检查TCP连接是否可建立 - 这是最基本的可用性检查
	conn, err := net.DialTimeout("tcp", ip, timeout)
	if err != nil {
		return false
	}
	conn.Close()

	// 解析代理URL
	proxyUrl, err := url.Parse("http://" + ip)
	if err != nil {
		return false
	}

	// 构建一个轻量级的HTTP请求
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyURL(proxyUrl),
			// 禁用HTTP/2以减少握手开销
			TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
			// 设置更激进的超时
			DialContext: (&net.Dialer{
				Timeout:   timeout,
				KeepAlive: timeout,
			}).DialContext,
			TLSHandshakeTimeout:   timeout,
			ResponseHeaderTimeout: timeout,
			ExpectContinueTimeout: 1 * time.Second,
			// 禁用压缩以减少处理时间
			DisableCompression: true,
		},
		Timeout: timeout,
	}

	// 请求简单的HEAD而非完整GET
	req, err := http.NewRequest("HEAD", "http://www.baidu.com", nil)
	if err != nil {
		return false
	}

	// 仅添加最必要的请求头
	req.Header.Set("User-Agent", "Mozilla/5.0")

	// 执行请求
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// 仅检查状态码
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

// CheckHttpResponse 🔄 验证代理的HTTP代理功能
//
// 通过代理向指定网站发送HTTP请求并验证响应
//
// 参数:
//   - ip: 代理服务器地址(格式："ip:port")
//   - reqDomain: 可选的测试目标域名，默认使用百度
//   - strContains: 可选的响应内容验证字符串
//
// 返回值:
//   - bool: 代理是否能成功完成HTTP请求
func CheckHttpResponse(ip, reqDomain, strContains string) bool {
	logger := logger.GetLogger()
	maxRetries := 1 // 减少重试次数
	var lastErr error

	for i := 0; i <= maxRetries; i++ {
		// 解析代理URL
		proxyUrl, err := url.Parse("http://" + ip)
		if err != nil {
			if i == maxRetries {
				logger.Debugf("❌ 解析代理URL错误 [%s]: %v", ip, err)
			}
			return false
		}

		// 确定测试目标网站
		testTarget := reqDomain
		if testTarget == "" {
			testTarget = "http://www.baidu.com"
		}

		// 设置请求头，模拟真实浏览器
		headers := http.Header{}
		headers.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

		// 准备HTTP请求
		req := netutil.HttpRequest{
			Method:  "GET",
			RawURL:  testTarget,
			Headers: headers,
		}

		// 配置HTTP客户端 - 减少超时时间
		clientCfg := netutil.HttpClientConfig{
			SSLEnabled: false,
			TLSConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
			Proxy:   proxyUrl,
			Timeout: 5 * time.Second, // 减少超时时间
		}

		// 发送请求并获取响应
		client := netutil.NewHttpClientWithConfig(&clientCfg)
		resp, err := client.SendRequest(&req)
		if err != nil {
			lastErr = err
			continue
		}

		// 读取响应内容
		defer resp.Body.Close()
		dataBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			lastErr = err
			continue
		}

		// 验证响应
		if len(dataBytes) > 0 {
			logger.Debugf("✅ HTTP代理验证成功: %s", ip)
			return true
		}
	}

	if lastErr != nil {
		logger.Debugf("❌ HTTP代理验证失败 [%s]: %v", ip, lastErr)
	}
	return false
}

// CheckHttpsResponse 🔒 验证代理的HTTPS代理功能
//
// 参数:
//   - ip: 代理服务器地址(格式："ip:port")
//   - reqDomain: 可选的测试目标域名，默认使用百度
//   - strContains: 可选的响应内容验证字符串
//
// 返回值:
//   - bool: 代理是否支持HTTPS连接
func CheckHttpsResponse(ip, reqDomain, strContains string) bool {
	// 使用更短的超时时间
	timeout := 4 * time.Second

	// 建立到代理服务器的TCP连接
	tcpConn, err := net.DialTimeout("tcp", ip, timeout)
	if err != nil {
		return false
	}
	defer tcpConn.Close()

	// 确定目标域名和端口
	targetDomain := "baidu.com"
	targetPort := "443"

	// 解析用户提供的目标
	if reqDomain != "" {
		domainParts := strings.Split(reqDomain, ":")
		targetDomain = domainParts[0]
		if len(domainParts) > 1 {
			targetPort = domainParts[1]
		}
	}

	// 规范化域名格式
	if !strings.HasPrefix(targetDomain, "www.") && !strings.Contains(targetDomain, ".") {
		targetDomain = "www." + targetDomain
	}

	// 构建完整目标地址
	targetAddress := targetDomain + ":" + targetPort

	// 构建简化的CONNECT请求
	connectReq := "CONNECT " + targetAddress + " HTTP/1.1\r\n" +
		"Host: " + targetAddress + "\r\n" +
		"User-Agent: Mozilla/5.0\r\n" +
		"Connection: keep-alive\r\n\r\n"

	// 设置写入超时
	tcpConn.SetWriteDeadline(time.Now().Add(timeout))
	_, err = tcpConn.Write([]byte(connectReq))
	if err != nil {
		return false
	}

	// 读取代理服务器响应
	buffer := make([]byte, 256) // 减小buffer大小
	tcpConn.SetReadDeadline(time.Now().Add(timeout))
	read, err := tcpConn.Read(buffer)
	if err != nil {
		return false
	}

	// 验证连接是否成功建立 - 只检查是否包含成功状态码
	response := string(buffer[:read])
	return strings.Contains(response, "200")
}

// CheckSocket5Response 🧦 验证代理的SOCKS5代理功能
//
// 参数:
//   - ip: 代理服务器地址(格式："ip:port")
//
// 返回值:
//   - bool: 代理是否支持SOCKS5协议
func CheckSocket5Response(ip string) bool {
	// 减少超时时间
	timeout := 4 * time.Second

	// 建立TCP连接
	destConn, err := net.DialTimeout("tcp", ip, timeout)
	if err != nil {
		return false
	}
	defer destConn.Close()

	// 设置写入超时
	destConn.SetWriteDeadline(time.Now().Add(timeout))

	// 发送SOCKS5握手请求
	// 0x05: SOCKS版本5
	// 0x01: 支持1种认证方法
	// 0x00: 无需认证的方法
	req := []byte{0x05, 0x01, 0x00}
	_, err = destConn.Write(req)
	if err != nil {
		return false
	}

	// 读取SOCKS5服务器响应
	bytes := make([]byte, 2) // 只需读取2字节即可判断
	destConn.SetReadDeadline(time.Now().Add(timeout))
	n, err := destConn.Read(bytes)
	if err != nil || n < 2 {
		return false
	}

	// 验证服务器响应
	return bytes[0] == 0x05 && bytes[1] != 0xFF
}

// CheckProxyAnonymity 🎭 检测代理的匿名性级别
//
// 参数:
//   - ip: 代理服务器地址(格式："ip:port")
//
// 返回值:
//   - string: 代理的匿名性级别
func CheckProxyAnonymity(ip string) string {
	// 减少超时时间
	timeout := 5 * time.Second

	// 解析代理URL
	proxyUrl, err := url.Parse("http://" + ip)
	if err != nil {
		return ""
	}

	// 配置HTTP客户端
	client := http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			Proxy:           http.ProxyURL(proxyUrl),
		},
	}

	// 创建请求
	request, err := http.NewRequest("GET", "http://httpbin.org/get", nil)
	if err != nil {
		return ""
	}
	request.Header.Add("Proxy-Connection", "keep-alive")

	// 发送请求
	res, err := client.Do(request)
	if err != nil {
		return ""
	}
	defer res.Body.Close()

	// 限制读取的数据量
	dataBytes, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	result := string(dataBytes)

	// 快速检查响应是否有效
	if !strings.Contains(result, `"url"`) {
		return ""
	}

	// 使用预编译的正则表达式检查IP泄露
	ipRegex := regexp.MustCompile(`(\d+?\.\d+?\.\d+?\.\d+?,.+\d+?\.\d+?\.\d+?\.\d+?)`)
	if ipRegex.FindString(result) != "" {
		return "transparent"
	}

	// 检查是否泄露了请求头
	if strings.Contains(result, "keep-alive") {
		return "common"
	}

	return "high"
}
