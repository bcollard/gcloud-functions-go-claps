package claps

import (
	"cloud.google.com/go/firestore"
	"context"
	"fmt"
	"github.com/gomodule/redigo/redis"
	"github.com/throttled/throttled"
	"github.com/throttled/throttled/store/redigostore"
	"google.golang.org/api/iterator"
	"log"
	"net/http"
	"os"
)

var count = 0
var redisPool *redis.Pool
var mux *http.ServeMux
var httpRateLimiter *throttled.HTTPRateLimiter
const REDIS_MAX_CONN = 10
var client *firestore.Client
const COLLECTION = "claps"

// function cold start init
func init() {
	mux = newMux()

}

func initializeRedis() (*redis.Pool, error) {
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

	return &redis.Pool{
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", redisAddr)
			if err != nil {
				return nil, fmt.Errorf("redis.Dial: %v", err)
			}
			return conn, err
		},
		MaxIdle: REDIS_MAX_CONN,
	}, nil
}

// custom HTTP Multiplexer
func newMux() *http.ServeMux {
	mux := http.NewServeMux()

	// RATE LIMITING
	quota := throttled.RateQuota{
		MaxRate: throttled.PerMin(2),
		MaxBurst: 5,
	}

	// Pre-declare err to avoid shadowing redisPool
	var err error
	redisPool, err = initializeRedis()
	if err != nil {
		log.Fatal(err)
	}

	store, err := redigostore.New(redisPool, "ip:", 0);
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
	}

	// MAIN ROUTER: apply rate-limiting to GET + POST methods and route request to dedicated function
	mux.Handle("/", httpRateLimiter.RateLimit(
		http.HandlerFunc(
			func(writer http.ResponseWriter, request *http.Request) {
				// CORS
				validCors, origin := validCors(request)
				if ! validCors {
					writer.WriteHeader(http.StatusForbidden)
					return
				} else {
					writer.Header().Set("Access-Control-Allow-Origin", origin)
				}

				// METHOD switch
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

	// API subpath
	mux.HandleFunc("/one", func(writer http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(writer, "%v", count)
	})

	return mux
}


func validCors(request *http.Request) (bool, string) {
	origin := request.Header.Get("Origin")
	whitelistOrigin := []string{"http://localhost", "https://www.baptistout.net"}
	var originAllowed bool

	for _, v := range whitelistOrigin {
		if v == origin {
			originAllowed = true
		}
	}

	return originAllowed, origin
}

func postClaps(writer http.ResponseWriter, request *http.Request) {
	fmt.Fprintf(writer, "post")
}

func getClaps(writer http.ResponseWriter, request *http.Request) {
	referrer := request.Header.Get("Referer")

	// FIRESTORE
	projectId := os.Getenv("PROJECT_ID")
	if projectId == "" {
		fmt.Println("PROJECT_ID must be set")
		os.Exit(1)
	}

	// connection
	// TODO: use a pooled connection when running in Google Functions env; see 'options' package
	ctx := context.Background()
	var err error
	client, err = firestore.NewClient(ctx, projectId)
	if err != nil {
		log.Fatalf("could not initialize Firestore client for project %s: %v", projectId, err)
	}
	defer client.Close()

	claps := client.Collection("claps")
	q := claps.Where("url", "==", referrer).Limit(1)

	iter := q.Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("could not iterate on results")
			break
		}
		claps, err := doc.DataAt("claps")
		if err != nil {
			log.Fatalf("could not get claps count")
			break
		}
		fmt.Fprintf(writer, "%v", claps)
	}


}


// Clapsgo is the Function entrypoint
func Clapsgo(w http.ResponseWriter, r *http.Request) {

	// use another http.ServeMux in order to do some routing by sub-paths
	mux.ServeHTTP(w, r)
}

