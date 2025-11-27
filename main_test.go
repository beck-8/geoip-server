package main

/*
GeoIP Server 性能测试

运行所有测试:
  go test -bench=. -benchmem

运行特定测试:
  go test -bench=BenchmarkQueryGeo -benchmem
  go test -bench=BenchmarkGeoHandler -benchmem
  go test -bench=BenchmarkCachePerformance -benchmem

生成性能分析文件:
  go test -bench=. -benchmem -cpuprofile=cpu.prof -memprofile=mem.prof

查看性能分析:
  go tool pprof cpu.prof
  go tool pprof mem.prof

测试并发性能:
  go test -bench=Parallel -benchmem

注意: 需要在当前目录下有 GeoLite2-City.mmdb 和 GeoLite2-ASN.mmdb 文件
*/

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/golang/groupcache/lru"
	"github.com/oschwald/geoip2-golang/v2"
)

// 初始化测试环境
func setupTest(b *testing.B) {
	b.Helper()

	var err error
	// 尝试加载 MaxMind 数据库
	countryDB, err = geoip2.Open("GeoLite2-City.mmdb")
	if err != nil {
		b.Skipf("Skipping test: GeoLite2-City.mmdb not found: %v", err)
	}

	asnDB, err = geoip2.Open("GeoLite2-ASN.mmdb")
	if err != nil {
		b.Skipf("Skipping test: GeoLite2-ASN.mmdb not found: %v", err)
	}

	geoCache = lru.New(10000)
	gin.SetMode(gin.ReleaseMode)
}

// 清理测试环境
func teardownTest(b *testing.B) {
	b.Helper()
	if countryDB != nil {
		countryDB.Close()
	}
	if asnDB != nil {
		asnDB.Close()
	}
}

// BenchmarkQueryGeo 测试 queryGeo 函数的性能
func BenchmarkQueryGeo(b *testing.B) {
	setupTest(b)
	defer teardownTest(b)

	ip, _ := netip.ParseAddr("8.8.8.8")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := queryGeo(ip)
		if err != nil {
			b.Fatalf("queryGeo failed: %v", err)
		}
	}
}

// BenchmarkQueryGeoWithCache 测试缓存命中时的性能
func BenchmarkQueryGeoWithCache(b *testing.B) {
	setupTest(b)
	defer teardownTest(b)

	ip, _ := netip.ParseAddr("8.8.8.8")
	// 预热缓存
	queryGeo(ip)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := queryGeo(ip)
		if err != nil {
			b.Fatalf("queryGeo failed: %v", err)
		}
	}
}

// BenchmarkQueryGeoMultipleIPs 测试多个不同 IP 的查询性能
func BenchmarkQueryGeoMultipleIPs(b *testing.B) {
	setupTest(b)
	defer teardownTest(b)

	ips := []string{
		"8.8.8.8",
		"1.1.1.1",
		"119.29.29.29",
		"114.114.114.114",
		"223.5.5.5",
	}

	parsedIPs := make([]netip.Addr, len(ips))
	for i, ipStr := range ips {
		parsedIPs[i], _ = netip.ParseAddr(ipStr)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ip := parsedIPs[i%len(parsedIPs)]
		_, _, err := queryGeo(ip)
		if err != nil {
			b.Fatalf("queryGeo failed: %v", err)
		}
	}
}

// BenchmarkGetRealIP 测试 getRealIP 函数的性能
func BenchmarkGetRealIP(b *testing.B) {
	gin.SetMode(gin.ReleaseMode)

	testCases := []struct {
		name string
		xff  string
	}{
		{"NoXFF", ""},
		{"SingleIP", "8.8.8.8"},
		{"MultipleIPs", "8.8.8.8, 1.1.1.1, 119.29.29.29"},
		{"WithPrivateIP", "192.168.1.1, 8.8.8.8, 1.1.1.1"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request, _ = http.NewRequest("GET", "/", nil)
			c.Request.RemoteAddr = "203.0.113.1:12345"
			if tc.xff != "" {
				c.Request.Header.Set("X-Forwarded-For", tc.xff)
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = getRealIP(c)
			}
		})
	}
}

// BenchmarkGeoHandler 测试完整 HTTP 处理器的性能
func BenchmarkGeoHandler(b *testing.B) {
	setupTest(b)
	defer teardownTest(b)

	r := gin.New()
	r.Use(requestIDMiddleware())
	r.GET("/api/ipinfo", geoHandler)

	testCases := []struct {
		name  string
		query string
	}{
		{"WithIPParam", "?ip=8.8.8.8"},
		{"WithIPParam_CN", "?ip=119.29.29.29"},
		{"WithIPParam_IPv6", "?ip=2001:4860:4860::8888"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				w := httptest.NewRecorder()
				req, _ := http.NewRequest("GET", "/api/ipinfo"+tc.query, nil)
				r.ServeHTTP(w, req)

				if w.Code != http.StatusOK {
					b.Fatalf("Expected status 200, got %d", w.Code)
				}
			}
		})
	}
}

// BenchmarkGeoHandlerParallel 测试并发情况下的性能
func BenchmarkGeoHandlerParallel(b *testing.B) {
	setupTest(b)
	defer teardownTest(b)

	r := gin.New()
	r.Use(requestIDMiddleware())
	r.GET("/api/ipinfo", geoHandler)

	b.RunParallel(func(pb *testing.PB) {
		ips := []string{"8.8.8.8", "1.1.1.1", "119.29.29.29"}
		i := 0
		for pb.Next() {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", fmt.Sprintf("/api/ipinfo?ip=%s", ips[i%len(ips)]), nil)
			r.ServeHTTP(w, req)
			i++
		}
	})
}

// BenchmarkJSONSerialization 测试 JSON 序列化性能
func BenchmarkJSONSerialization(b *testing.B) {
	res := GeoResponse{
		IP:                    "8.8.8.8",
		ContinentCode:         "NA",
		Country:               "United States",
		CountryZH:             "美国",
		CountryCode:           "US",
		City:                  "Mountain View",
		CityZH:                "芒廷维尤",
		RegisteredCountryCode: "US",
		ASN:                   15169,
		Organization:          "Google LLC",
		Timestamp:             1234567890,
		RequestID:             "test-request-id",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := json.Marshal(res)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCachePerformance 测试缓存性能
func BenchmarkCachePerformance(b *testing.B) {
	cache := lru.New(10000)
	entry := &geoCacheEntry{
		country: &geoip2.City{},
		asn:     &geoip2.ASN{},
	}

	b.Run("Add", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.Add(fmt.Sprintf("192.168.1.%d", i%256), entry)
		}
	})

	b.Run("Get_Hit", func(b *testing.B) {
		// 预填充缓存
		for i := 0; i < 1000; i++ {
			cache.Add(fmt.Sprintf("192.168.1.%d", i), entry)
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.Get(fmt.Sprintf("192.168.1.%d", i%1000))
		}
	})

	b.Run("Get_Miss", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			cache.Get(fmt.Sprintf("10.0.0.%d", i))
		}
	})
}
