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
var redisPool *redis.Pool
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
	redisPool = &redis.Pool{
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

	store, err := redigostore.New(redisPool, "ip:", 0);
	if err != nil {
		log.Fatal(err)
	}

	ratelimiter, err := throttled.NewGCRARateLimiter(store, quota)
	if err != nil {
		log.Fatal(err)
	}

	//headers := []string{"x-forwarded-for"}

	httpRateLimiter = &throttled.HTTPRateLimiter{
		RateLimiter: ratelimiter,
		//VaryBy:      &throttled.VaryBy{Headers: headers},
		VaryBy:      &throttled.VaryBy{RemoteAddr: true},
	}

	mux.Handle("/", httpRateLimiter.RateLimit(mux)) // commenté car doublon register sur '/'
	//httpRateLimiter.RateLimit(mux)

	// route mapping
	mux.HandleFunc("/one", func(writer http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(writer, "%v", count)
	})

	//mux.HandleFunc("/", func(writer http.ResponseWriter, r *http.Request) {
	//	fmt.Fprintf(writer, "slash")
	//})

	return mux
}


// Clapsgo prints a figure incrementally
func Clapsgo(w http.ResponseWriter, r *http.Request) {

	count++

	w.Header().Set("Access-Control-Allow-Origin", "*")
	conn := redisPool.Get()
	defer conn.Close()

	// use another http.ServeMux in order to do some routing by sub-paths
	mux.ServeHTTP(w, r)

	fmt.Fprintf(w, "main")

	//switch r.Method {
	//case http.MethodGet:
	//	fmt.Fprint(w, count)
	//case http.MethodPost:
	//	http.Error(w, "403 - Forbidden", http.StatusForbidden)
	//default:
	//	http.Error(w, "405 - Method Not Allowed", http.StatusMethodNotAllowed)
	//}

}

