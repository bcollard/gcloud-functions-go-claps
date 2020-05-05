package claps

import (
	"cloud.google.com/go/firestore"
	"context"
	"fmt"
	"github.com/gomodule/redigo/redis"
	"github.com/throttled/throttled"
	"github.com/throttled/throttled/store/redigostore"
	"log"
	"net/http"
	"os"
	"regexp"
)

var count = 0
var redisPool *redis.Pool
var mux *http.ServeMux
var httpRateLimiter *throttled.HTTPRateLimiter
const REDIS_MAX_CONN = 10
var client *firestore.Client
const COLLECTION = "claps"
var PROJECT_ID = ""
var ReferrerWhitelistRegex = `^https:\/\/www.baptistout.net\/posts\/[\w\d-]+\/?$`
var referrerRegexp *regexp.Regexp
var CorsWhitelist = []string{"https://www.baptistout.net"}
var OauthRedirectUri = "http://localhost:8080/secure/oauthcallback"
var ipCountGetMap, ipCountPostMap map[string]int
const MAX_GET_PER_IP = 1000;
const MAX_POST_PER_IP = 200;


// function cold start init
func init() {
	mux = newMux()

	PROJECT_ID = os.Getenv("PROJECT_ID")
	if PROJECT_ID == "" {
		fmt.Println("PROJECT_ID must be set")
		os.Exit(1)
	}

	if os.Getenv("FIRESTORE_ENV") == "local" {
		CorsWhitelist = append(CorsWhitelist,"http://localhost:1313")
		referrerRegexp, _ = regexp.Compile(`^http:\/\/localhost:1313\/posts\/[\w\d-]+\/?$`)
	} else {
		OauthRedirectUri = os.Getenv("OauthRedirectUri")
		referrerRegexp, _ = regexp.Compile(ReferrerWhitelistRegex)
	}

	ipCountGetMap = make(map[string]int, 10)
	ipCountPostMap = make(map[string]int, 10)
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

				// REFERRER
				validReferrer, referrer := validReferrer(request)
				if referrer != "" && ! validReferrer {
					writer.WriteHeader(http.StatusForbidden)
					return
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

	for _, v := range CorsWhitelist {
		if v == origin {
			return true, origin
		}
	}

	return false, origin
}

func validReferrer(request *http.Request) (bool, string) {
	referrer := request.Header.Get("Referer")
	return referrerRegexp.MatchString(referrer), referrer
}

// POST
func postClaps(writer http.ResponseWriter, request *http.Request) {
	referrer := request.Header.Get("Referer")
	ip := request.Header.Get("x-forwarded-for")

	// REQUEST COUNT LIMIT
	ipCount, prs := ipCountPostMap[ip]
	if prs && ipCount > MAX_POST_PER_IP {
		writer.WriteHeader(http.StatusTooManyRequests)
		return
	} else {
		if prs {
			ipCountPostMap[ip] = ipCount + 1
		} else {
			ipCountPostMap[ip] = 1
		}
	}

	// FIRESTORE CONNECTION
	// TODO: use a connection pool; see 'google.golang.org/api/option' package
	ctx := context.Background()
	var err error
	client, err = firestore.NewClient(ctx, PROJECT_ID)
	if err != nil {
		log.Fatalf("could not initialize Firestore client for project %s: %v", PROJECT_ID, err)
	}
	defer client.Close()

	claps := client.Collection("claps")
	q := claps.Where("url", "==", referrer).Limit(1)

	documentSnapshot, err := q.Documents(ctx).Next()
	if err != nil {
		fmt.Printf("could not iterate: %v", err)
		// ADD new doc
		claps.Add(ctx, map[string]interface{}{
			"url": referrer,
			"claps": 1,
		})
	} else {
		// UPDATE doc
		rawClaps, err := documentSnapshot.DataAt("claps")
		if err != nil {
			fmt.Printf("could not get claps count, %v", err)
		}
		documentSnapshot.Ref.Update(ctx, []firestore.Update{{Path: "claps", Value: rawClaps.(int64) + 1}})
	}
}

// GET
func getClaps(writer http.ResponseWriter, request *http.Request) {
	referrer := request.Header.Get("Referer")
	ip := request.Header.Get("x-forwarded-for")

	// REQUEST COUNT LIMIT
	ipCount, prs := ipCountGetMap[ip]
	if prs && ipCount > MAX_GET_PER_IP {
		writer.WriteHeader(http.StatusTooManyRequests)
		return
	} else {
		if prs {
			ipCountGetMap[ip] = ipCount + 1
		} else {
			ipCountGetMap[ip] = 1
		}
	}

	// FIRESTORE connection
	// TODO: use a connection pool; see 'google.golang.org/api/option' package
	ctx := context.Background()
	var err error
	client, err = firestore.NewClient(ctx, PROJECT_ID)
	if err != nil {
		log.Fatalf("could not initialize Firestore client for project %s: %v", PROJECT_ID, err)
	}
	defer client.Close()

	claps := client.Collection("claps")
	q := claps.Where("url", "==", referrer).Limit(1)

	documentSnapshot, err := q.Documents(ctx).Next()
	if err != nil {
		// doesn't exist, returns 0
		fmt.Fprint(writer,0)
	} else {
		clapsCount, err := documentSnapshot.DataAt("claps")
		if err != nil {
			fmt.Printf("could not get claps count, %v", err)
		}
		fmt.Fprintf(writer, "%v", clapsCount)
	}
}


// Clapsgo is the Function entrypoint
func Clapsgo(w http.ResponseWriter, r *http.Request) {

	// use another http.ServeMux in order to do some routing by sub-paths
	mux.ServeHTTP(w, r)
}

