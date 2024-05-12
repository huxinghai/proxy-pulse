package main

import (
    "errors"
    "io"
    "log"
    "net/http"
    "net/url"
    "sync"
    "time"
)

// ProxyPool 是可用代理的列表
var (
    ProxyPool []string
    mu        sync.Mutex
)

// 验证代理服务器的有效性
func validateProxy(proxyAddress string) bool {
    // 此处仅提供一个简单的http请求测试，实际验证可能需要更复杂的逻辑
    url, err := url.Parse(proxyAddress)
    if err != nil {
        log.Println(err)
        return false
    }

    client := &http.Client{
        Transport: &http.Transport{
            Proxy: http.ProxyURL(url),
        },
        Timeout: 5 * time.Second,
    }

    resp, err := client.Get("http://www.example.com")
    if err != nil {
        log.Println(err)
        return false
    }
    defer resp.Body.Close()

    return resp.StatusCode == http.StatusOK
}

// 更新代理服务器列表
func updateProxyList(newProxies []string) {
    mu.Lock()
    defer mu.Unlock()

    // 验证代理服务器，只添加有效的代理到列表中
    validProxies := make([]string, 0)
    for _, proxyAddress := range newProxies {
        if validateProxy(proxyAddress) {
            validProxies = append(validProxies, proxyAddress)
        }
    }

    ProxyPool = validProxies
}

// selectProxy 随机或者按某种策略选择一个可用的代理，这里仅返回第一个作为示例
func selectProxy() (*url.URL, error) {
    mu.Lock()
    defer mu.Unlock()

    if len(ProxyPool) == 0 {
        return nil, errors.New("no proxies available")
    }

    // 实际应用中可能需要添加更复杂的选择逻辑
    return url.Parse(ProxyPool[0])
}

// BasicAuth 中间件进行HTTP基本认证
func BasicAuth(handler http.HandlerFunc, username, password string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        user, pass, ok := r.BasicAuth()
        if !ok || user != username || pass != password {
            w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        handler(w, r)
    }
}

func handleRequestAndRedirect(resp http.ResponseWriter, req *http.Request) {
    proxyURL, err := selectProxy()
    if err != nil {
        http.Error(resp, "Failed to select proxy", http.StatusBadGateway)
        return
    }

    // 修改请求以指向代理
    req.URL.Scheme = proxyURL.Scheme
    req.URL.Host = proxyURL.Host

    // 创建转发请求的Transport
    transport := &http.Transport{
        Proxy: http.ProxyURL(proxyURL),
    }
    client := &http.Client{
        Transport: transport,
        Timeout:   10 * time.Second,
    }

    // 请求复制
    outReq := new(http.Request)
    *outReq = *req

    // 发送请求
    outResp, err := client.Do(outReq)
    if err != nil {
        http.Error(resp, "Failed to forward request", http.StatusBadGateway)
        return
    }
    defer outResp.Body.Close()

    // 复制响应头
    for key, value := range outResp.Header {
        for _, v := range value {
            resp.Header().Add(key, v)
        }
    }
    resp.WriteHeader(outResp.StatusCode)
    io.Copy(resp, outResp.Body)
}

func main() {
    // For example, you might want to update the proxy list every 10 minutes.
    go func() {
        for {
            // Supposedly you would fetch this list from a database or some API
            updatedList := []string{
                "http://newproxy1.example.com:8080",
                "http://newproxy2.example.com:8080",
                // ...
            }
            updateProxyList(updatedList)
            time.Sleep(10 * time.Minute)
        }
    }()

    username := "user"
    password := "pass"
    http.HandleFunc("/", BasicAuth(handleRequestAndRedirect, username, password))
    log.Println("Starting proxy server on :8888")
    log.Fatal(http.ListenAndServeTLS(":8888", "server.crt", "server.key", nil))
}