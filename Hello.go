package claps

import (
	"fmt"
	"github.com/gomodule/redigo/redis"
	"github.com/throttled/throttled"
	"github.com/throttled/throttled/store/redigostore"
	"log"
	"net/http"
	"os"
)

var count = 0
var redisPool redis.Pool
var mux = newMux()
var httpRateLimiter *throttled.HTTPRateLimiter

// function cold start init
func init() {

	// REDIS RATE LIMITING
	redisHost := os.Getenv("REDIS_HOST")
	if redisHost == "" {
		fmt.Println("REDIS_HOST must be set")
		os.Exit(1)
	}
	redisPort := os.Getenv("REDIS_PORT")
	if redisPort == "" {
		fmt.Println("REDIS_PORT must be set")
		os.Exit(1)
	}

	redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)

	const maxConnections = 10
	redisPool = redis.Pool{
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", redisAddr)
			if err != nil {
				return nil, fmt.Errorf("redis.Dial: %v", err)
			}
			return conn, err
		},
		MaxIdle: maxConnections,
	}

}

// custom HTTP Multiplexer
func newMux() *http.ServeMux {
	mux := http.NewServeMux()

	// rate limiter middleware
	quota := throttled.RateQuota{
		MaxRate: throttled.PerMin(2),
		MaxBurst: 5,
	}

	store, err := redigostore.New(&redisPool, "ip:", 0);
	if err != nil {
		log.Fatal(err)
	}

	ratelimiter, err := throttled.NewGCRARateLimiter(store, quota)
	if err != nil {
		log.Fatal(err)
	}

	headers := []string{"x-forwarded-for"}

	httpRateLimiter = &throttled.HTTPRateLimiter{
		RateLimiter: ratelimiter,
		VaryBy:      &throttled.VaryBy{Headers: headers},
		//VaryBy:      &throttled.VaryBy{RemoteAddr: true},
	}

	// main router: apply rate-limiting to GET + POST methods and route request to dedicated functions
	mux.Handle("/", httpRateLimiter.RateLimit(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method  {
		case http.MethodGet:
			getClaps(writer, request)
			break
		case http.MethodPost:
			postClaps(writer, request)
			break
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
	})))

	// route mapping
	mux.HandleFunc("/one", func(writer http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(writer, "%v", count)
	})

	//mux.HandleFunc("/", func(writer http.ResponseWriter, r *http.Request) {
	//	fmt.Fprintf(writer, "slash")
	//})

	return mux
}

func postClaps(writer http.ResponseWriter, request *http.Request) {
	fmt.Fprintf(writer, "post")
}

func getClaps(writer http.ResponseWriter, request *http.Request) {
	fmt.Fprintf(writer, "get")
}


// Clapsgo is the Function entrypoint
func Clapsgo(w http.ResponseWriter, r *http.Request) {

	count++

	// CORS
	origin := r.Header.Get("Origin")
	whitelistOrigin := []string{"http://localhost", "https://www.baptistout.net"}
	var originAllowed bool

	for _, v := range whitelistOrigin {
		if v == origin {
			originAllowed = true
		}
	}

	if originAllowed {
		w.Header().Set("Access-Control-Allow-Origin", origin)
	} else {
		w.WriteHeader(http.StatusForbidden)
	}

	// get one connection from the Redis pool per request
	conn := redisPool.Get()
	defer conn.Close()

	// use another http.ServeMux in order to do some routing by sub-paths
	mux.ServeHTTP(w, r)

	//fmt.Fprintf(w, "main")

}

