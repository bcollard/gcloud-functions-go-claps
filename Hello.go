package claps

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gomodule/redigo/redis"
)

var count = 0
var redisPool *redis.Pool

// function cold start init
func init() {
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

// Clapsgo prints a figure incrementally
func Clapsgo(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Access-Control-Allow-Origin", "*")

	conn := redisPool.Get()
	defer conn.Close()

	counter, err := redis.Int(conn.Do("INCR", "visits"))
	if err != nil {
		log.Printf("redis.Int: %v", err)
		http.Error(w, "Error incrementing visit count", http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "Visit count: %d", counter)


}

