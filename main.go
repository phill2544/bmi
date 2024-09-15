package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/joho/godotenv"
	"github.com/labstack/gommon/log"
	"go.uber.org/zap"
)

var (
	cors_fiber string
	fiber_port string
	// db_database    string
	// db_host        string
	// db_password    string
	// db_port        string
	// db_schema      string
	// db_sslmod      string
	// db_username    string
	rate_limit     string
	time_limit     string
	redis_host     string
	redis_port     string
	redis_password string

	redis_client *redis.Client
)

type BMIRequest struct {
	Weight float64 `json:"weight"`
	Height float64 `json:"height"`
}

func main() {
	if err := godotenv.Load("./config.env"); err != nil {
		log.Error("Error loading config file", zap.Error(err))
	}
	cors_fiber = os.Getenv("CORS_ALLOW_ORIGIN")
	fiber_port = os.Getenv("FIBER_PORT")
	// db_database = os.Getenv("DB_DATABASE")
	// db_host = os.Getenv("DB_HOST")
	// db_password = os.Getenv("DB_PASSWORD")
	// db_port = os.Getenv("DB_PORT")
	// db_schema = os.Getenv("DB_SCHEMA")
	// db_sslmod = os.Getenv("DB_SSLMOD")
	// db_username = os.Getenv("DB_USERNAME")
	rate_limit = os.Getenv("RATE_LIMIT")
	time_limit = os.Getenv("TIME_LIMIT")
	redis_host = os.Getenv("REDIS_HOST")
	redis_port = os.Getenv("REDIS_PORT")
	redis_password = os.Getenv("REDIS_PASSWORD")

	rate_limit, err := strconv.Atoi(rate_limit)
	if err != nil {
		log.Error("Error converting string to int:", err)
		return
	}
	time_limit, err := strconv.Atoi(time_limit)
	if err != nil {
		log.Error("Error converting string to int:", err)
		return
	}
	fiber_port, err := strconv.Atoi(fiber_port)
	if err != nil {
		log.Error("Error converting string to int:", err)
		return
	}
	redis_client = initRedis()

	app := fiber.New()
	app.Use(cors.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: cors_fiber,
		AllowMethods: strings.Join([]string{
			fiber.MethodGet,
			fiber.MethodPost,
			fiber.MethodHead,
			fiber.MethodPut,
			fiber.MethodDelete,
			fiber.MethodPatch,
		}, ","),
		AllowHeaders: "Content-Type, Authorization",
	}))
	app.Use(logHTTPMethod)
	app.Use(setRateLimit(rate_limit, time_limit))
	app.Post("/bmi", calculateBMI)

	err = app.Listen(fmt.Sprintf(":%v", fiber_port))
	if err != nil {
		log.Error(fmt.Sprintf("Listen To Port %v %v", fiber_port, err))
	}
}

// func initDatabase() *gorm.DB {
// 	var db *gorm.DB
// 	dsn := fmt.Sprintf("host=%v user=%v password=%v dbname=%v port=%v sslmode=%v search_path=%s",
// 		db_host,
// 		db_username,
// 		db_password,
// 		db_database,
// 		db_port,
// 		db_sslmod,
// 		db_schema,
// 	)

// 	dial := postgres.New(postgres.Config{DSN: dsn})

// 	db, err := gorm.Open(dial, &gorm.Config{
// 		Logger: logger.Default.LogMode(logger.Silent),
// 	})
// 	if err != nil {
// 		log.Error(fmt.Sprintf("Error connecting to the database : %v", err), zap.Error(err))
// 		panic(err)
// 	} else {
// 		log.Info("Connected to the database")
// 	}
// 	return db
// }

func logHTTPMethod(c *fiber.Ctx) error {
	log.Info("HTTP method used", zap.String("method", c.Method()))
	return c.Next()
}
func setRateLimit(request int, expireTime int) fiber.Handler {
	limitConfig := limiter.New(limiter.Config{
		Max:        request,                                 // Maximum number of requests
		Expiration: time.Duration(expireTime) * time.Second, // Time window for rate limiting
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP() // Use client's IP address as the key
		},
		LimitReached: func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"message": "Too many requests please try again later",
			})
		},
	})
	return limitConfig
}

func initRedis() *redis.Client {

	client := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%v:%v", redis_host, redis_port),
		Password: fmt.Sprintf("%v", redis_password),
	})

	_, err := client.Ping().Result()
	if err != nil {
		log.Error("can not connect redis")
		panic(err)
	} else {
		log.Info("Connected to Redis")
	}

	return client
}

func calculateBMI(c *fiber.Ctx) error {
	reqBody := BMIRequest{}
	var redis_data interface{}
	err := c.BodyParser(&reqBody)
	if err != nil {
		log.Error("invalid request" + err.Error())
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid input data",
		})
	}
	if reqBody.Height == 0 {
		log.Error("invalid height")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid height",
		})
	}
	if reqBody.Weight == 0 {
		log.Error("invalid weight")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid weight",
		})
	}

	key := fmt.Sprintf("bmi:%v:%v", reqBody.Height, reqBody.Weight)
	//redis get
	res, err := redis_client.Get(key).Result()
	if err == nil && res != "" {
		if json.Unmarshal([]byte(res), &redis_data) == nil {
			return c.Status(fiber.StatusOK).JSON(fiber.Map{
				"BMI":     redis_data,
				"message": bmiMessage(redis_data.(float64)),
			})
		}
	}
	heightMeter := reqBody.Height / 100
	respBMI := reqBody.Weight / (heightMeter * heightMeter)
	data, err := json.Marshal(respBMI)
	if err == nil {
		redis_client.Set(key, string(data), time.Second*500)
	} else {
		log.Error(err)
	}
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"BMI":     respBMI,
		"message": bmiMessage(respBMI),
	})
}

func bmiMessage(bmi float64) string {
	switch {
	case bmi < 18.5:
		return "Underweight"
	case bmi >= 18.5 && bmi < 24.9:
		return "Normal weight"
	case bmi >= 25 && bmi < 29.9:
		return "Overweight"
	case bmi >= 30 && bmi < 34.9:
		return "Obesity class I"
	case bmi >= 35 && bmi < 39.9:
		return "Obesity class II"
	case bmi >= 40:
		return "Obesity class III"
	default:
		return "Invalid BMI"
	}
}
