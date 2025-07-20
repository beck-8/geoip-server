package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang/groupcache/lru"
	"github.com/google/uuid"
	"github.com/natefinch/lumberjack"
	"github.com/oschwald/geoip2-golang/v2"
)

var (
	countryDB *geoip2.Reader
	asnDB     *geoip2.Reader
	geoCache  *lru.Cache
	asnCache  *lru.Cache
)

type GeoResponse struct {
	IP                    string `json:"ip"`
	ContinentCode         string `json:"continent_code"`
	Country               string `json:"country"`
	CountryZH             string `json:"country_zh"`
	CountryCode           string `json:"country_code"`
	RegisteredCountryCode string `json:"registered_country_code"`
	ASN                   uint   `json:"asn"`
	Organization          string `json:"organization"`
	ASNIPv4Num            uint   `json:"asn_ipv4_num"`
	Timestamp             int64  `json:"timestamp"`
	RequestID             string `json:"request_id"`
}

func getRealIP(c *gin.Context) string {
	xff := c.GetHeader("X-Forwarded-For")
	if xff != "" {
		for _, ip := range strings.Split(xff, ",") {
			ip = strings.TrimSpace(ip)
			parsed := net.ParseIP(ip)
			if parsed != nil && !parsed.IsPrivate() && !parsed.IsLoopback() {
				return ip
			}
		}
	}
	ip, _, _ := net.SplitHostPort(c.Request.RemoteAddr)
	return ip
}

type geoCacheEntry struct {
	country *geoip2.Country
	asn     *geoip2.ASN
}

func queryGeo(ip netip.Addr) (*geoip2.Country, *geoip2.ASN, error) {
	if v, ok := geoCache.Get(ip.String()); ok {
		entry := v.(*geoCacheEntry)
		return entry.country, entry.asn, nil
	}

	countryRecord, err := countryDB.Country(ip)
	if err != nil {
		return nil, nil, err
	}

	asnRecord, err := asnDB.ASN(ip)
	if err != nil {
		return countryRecord, nil, err
	}

	geoCache.Add(ip.String(), &geoCacheEntry{country: countryRecord, asn: asnRecord})

	return countryRecord, asnRecord, nil
}

func geoHandler(c *gin.Context) {
	queryIP := c.Query("ip")
	var ipStr string

	if queryIP != "" {
		ipStr = queryIP
	} else {
		ipStr = getRealIP(c)
	}

	ip, err := netip.ParseAddr(ipStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid IP"})
		return
	}

	cityRecord, asnRecord, err := queryGeo(ip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "GeoIP lookup failed"})
		return
	}

	requestID, _ := c.Get("RequestID")
	res := GeoResponse{
		IP:                    ip.String(),
		ContinentCode:         cityRecord.Continent.Code,
		Country:               cityRecord.Country.Names.English,
		CountryZH:             cityRecord.Country.Names.SimplifiedChinese,
		CountryCode:           cityRecord.Country.ISOCode,
		RegisteredCountryCode: cityRecord.RegisteredCountry.ISOCode,
		Timestamp:             time.Now().UnixMilli(),
		RequestID:             requestID.(string),
	}

	if asnRecord != nil {
		res.ASN = asnRecord.AutonomousSystemNumber
		res.Organization = asnRecord.AutonomousSystemOrganization
		res.ASNIPv4Num = asnRecord.AutonomousSystemNumber
	}

	c.JSON(http.StatusOK, res)
}

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.Request.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set("RequestID", requestID)
		c.Writer.Header().Set("X-Request-ID", requestID)
		c.Next()
	}
}

func init() {
	if strings.ToLower(os.Getenv("MAXMIND_PPROF")) != "" {
		go func() {
			log.Println("Starting pprof server on :62000")
			if err := http.ListenAndServe(":62000", nil); err != nil {
				log.Fatal("Failed to start pprof server: ", err)
			}
		}()
	}
}

func main() {
	cityMMDBPath := flag.String("country-mmdb", "GeoLite2-Country.mmdb", "Path to GeoLite2-Country.mmdb")
	asnMMDBPath := flag.String("asn-mmdb", "GeoLite2-ASN.mmdb", "Path to GeoLite2-ASN.mmdb")
	port := flag.String("port", ":8399", "HTTP server port")
	cacheSize := flag.Int("cache", 10000, "Number of LRU cache entries")
	logPath := flag.String("log", "geo.log", "Log file path")
	logSize := flag.Int("logsize", 10, "Max size (MB) per log file")
	logBackups := flag.Int("logbackups", 5, "Number of backup logs to retain")
	logAge := flag.Int("logage", 14, "Max age (days) to retain logs")
	flag.Parse()

	multiWriter := io.MultiWriter(os.Stdout, &lumberjack.Logger{
		Filename:   *logPath,
		MaxSize:    *logSize,
		MaxBackups: *logBackups,
		MaxAge:     *logAge,
		Compress:   true,
	})
	gin.DefaultWriter = multiWriter

	geoCache = lru.New(*cacheSize)

	var err error
	countryDB, err = geoip2.Open(*cityMMDBPath)
	if err != nil {
		log.Fatalf("Failed to open city mmdb: %v", err)
	}
	defer countryDB.Close()

	asnDB, err = geoip2.Open(*asnMMDBPath)
	if err != nil {
		log.Fatalf("Failed to open ASN mmdb: %v", err)
	}
	defer asnDB.Close()

	r := gin.New()

	// Custom logger formatter
	r.Use(gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		requestID, _ := param.Keys["RequestID"].(string)
		return fmt.Sprintf("[%s] %s - [%s] \"%s %s %s\" %d %d \"%s\" \"%s\" \"%s\" \"%s\" \"%s\"\n",
			param.TimeStamp.Format(time.RFC3339),
			param.ClientIP,
			requestID,
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency.Microseconds(),
			param.Request.Host,
			param.Request.UserAgent(),
			param.Request.Header.Get("X-Forwarded-For"),
			param.Request.Header.Get("X-Real-IP"),
			param.Request.RemoteAddr,
		)
	}), gin.Recovery())

	r.Use(requestIDMiddleware())

	api := r.Group("/api")
	api.GET("/ipinfo", geoHandler)
	r.Run(*port)
}
