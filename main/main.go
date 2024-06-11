package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-gomail/gomail"
	"github.com/streadway/amqp"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Pfc struct {
	id      string
	Protein float64 `json:"protein"`
	Fats    float64 `json:"fats"`
	Carbs   float64 `json:"carbs"`
	Date    string  `json:"date"`
	Name    string  `json:"name"`
	Email   string  `json:"email"`
}

func main() {
	go scheduleJob()
	startRegularTask()
	router := gin.Default()
	router.POST("/ping", func(c *gin.Context) {
		var pfc Pfc
		err := c.BindJSON(&pfc)
		if err != nil {
			fmt.Println(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка чтения JSON"})
			return
		}

		// Проверяем, существует ли запись с указанной датой в базе данных
		existingPfcs, err := getByDate(pfc.Date)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Внутренняя ошибка сервера"})
			return
		}

		if len(existingPfcs) > 0 {
			existingPfc := existingPfcs[0]
			existingPfc.Protein += pfc.Protein
			existingPfc.Fats += pfc.Fats
			existingPfc.Carbs += pfc.Carbs
			existingPfc.Name = pfc.Name
			existingPfc.Email = pfc.Email

			// Обновляем запись в базе данных
			err := mongoUpdater(existingPfc)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось обновить запись в базе данных"})
				return
			}

			c.JSON(http.StatusOK, "Показатели обновлены")
			return
		}

		err = mongoWriter(pfc)
		if err != nil {
			fmt.Println(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось добавить новую запись в базу данных"})
			return
		}
		c.JSON(http.StatusOK, "Новая запись добавлена")

	})

	router.GET("/get", func(c *gin.Context) {
		var pfc Pfc
		err := c.BindJSON(&pfc)
		if err != nil {
			fmt.Println(err)
			return
		}
		res, err := getByDate(pfc.Date)
		if err != nil {
			return
		}
		c.JSON(http.StatusOK, res)
	})
	router.DELETE("/delete", func(c *gin.Context) {
		var pfc Pfc
		err := c.BindJSON(&pfc)
		if err != nil {
			fmt.Println(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Ошибка чтения JSON"})
			return
		}
		err = deleteByDate(pfc.Date)
		if err != nil {
			fmt.Println(err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Не удалось удалить запись из базы данных"})
			return
		}

		c.JSON(http.StatusOK, "Запись удалена")
	})
	err := router.Run(":9888")
	if err != nil {
		log.Fatalf("[Error] failed to start Gin server due to: %v", err)
	}
}

func mongoWriter(pfc Pfc) error {
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		return err
	}
	defer client.Disconnect(context.TODO())

	usersCollection := client.Database("pfc").Collection("pfc")
	pfcWrite := bson.D{
		{"protein", pfc.Protein}, {"fats", pfc.Fats},
		{"carbs", pfc.Carbs}, {"name", pfc.Name}, {"date", pfc.Date}, {"email", pfc.Email}}
	result, err := usersCollection.InsertOne(context.TODO(), pfcWrite)
	if err != nil {
		return err
	}
	fmt.Println(result.InsertedID)
	return nil
}

func getByDate(date string) ([]Pfc, error) {
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		return nil, err
	}
	usersCollection := client.Database("pfc").Collection("pfc")
	filter := bson.D{
		{"date", date},
	}
	var results []Pfc
	cursor, err := usersCollection.Find(context.TODO(), filter)
	// check for errors in the finding
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.TODO())

	for cursor.Next(context.TODO()) {
		var result Pfc
		err := cursor.Decode(&result)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err := cursor.Err(); err != nil {
		return nil, err
	}
	return results, nil
}
func mongoUpdater(pfc Pfc) error {
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		return err
	}
	defer client.Disconnect(context.TODO())

	usersCollection := client.Database("pfc").Collection("pfc")
	filter := bson.D{{"date", pfc.Date}}

	update := bson.D{
		{"$set", bson.D{
			{"protein", pfc.Protein},
			{"fats", pfc.Fats},
			{"carbs", pfc.Carbs},
			{"name", pfc.Name},
			{"email", pfc.Email},
		}},
	}

	_, err = usersCollection.UpdateOne(context.TODO(), filter, update)
	if err != nil {
		return err
	}

	return nil
}

// Функция для удаления записи из базы данных по указанной дате
func deleteByDate(date string) error {
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		return err
	}
	defer client.Disconnect(context.TODO())

	usersCollection := client.Database("pfc").Collection("pfc")
	filter := bson.D{
		{"date", date},
	}

	_, err = usersCollection.DeleteOne(context.TODO(), filter)
	if err != nil {
		return err
	}

	return nil
}

func scheduleJob() error {
	fmt.Println("Trying to connect to RabbitMQ...")

	rabbitURI := "amqp://guest:guest@localhost:5672/"
	rabbitConn, err := amqp.Dial(rabbitURI)
	if err != nil {
		return err
	}
	defer rabbitConn.Close()

	ch, err := rabbitConn.Channel()
	if err != nil {
		return err

	}

	defer ch.Close()

	// Создание очереди в RabbitMQ
	queueName := "client_info"
	_, err = ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		return nil
	}

	// Запуск шедулера
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	fmt.Println("Scheduler started...")
	for range ticker.C {
		uniqueClients, err := getUniqueClients()
		if err != nil {
			log.Printf("Ошибка при получении уникальных клиентов из MongoDB: %v", err)
			continue
		}
		fmt.Println("Sending messages to RabbitMQ...")

		// Отправка данных в RabbitMQ
		for _, client := range uniqueClients {
			message := fmt.Sprintf("Имя: %s, Email: %s", client.Name, client.Email)
			err := ch.Publish(
				"",        // exchange
				queueName, // routing key
				false,     // mandatory
				false,     // immediate
				amqp.Publishing{
					ContentType: "text/plain",
					Body:        []byte(message),
				})
			if err != nil {
				log.Printf("Ошибка при отправке сообщения в RabbitMQ: %v", err)
				continue
			}
			log.Printf("Сообщение отправлено в RabbitMQ: %s", message)
		}
	}
	return nil
}

func getUniqueClients() ([]Pfc, error) {
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		return nil, err
	}
	defer client.Disconnect(context.TODO())

	usersCollection := client.Database("pfc").Collection("pfc")
	fmt.Println("Getting unique clients from MongoDB...")

	pipeline := mongo.Pipeline{
		{{"$group", bson.D{{"_id", "$name"}, {"email", bson.D{{"$first", "$email"}}}}}},
	}
	cursor, err := usersCollection.Aggregate(context.Background(), pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var uniqueClients []Pfc
	for cursor.Next(context.Background()) {
		var result struct {
			Name  string `bson:"Name"`
			Email string `bson:"email"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, err
		}
		uniqueClients = append(uniqueClients, Pfc{Name: result.Name, Email: result.Email})
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return uniqueClients, nil
}
func startRegularTask() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			consumeMessages() // Добавляем вызов функции для чтения сообщений
			log.Println("Regular task executed")
		}
	}
}
func consumeMessages() {
	fmt.Println("Listening for messages from RabbitMQ...")

	rabbitURI := "amqp://guest:guest@localhost:5672/"
	rabbitConn, err := amqp.Dial(rabbitURI)
	if err != nil {
		log.Fatal(err)
	}
	defer rabbitConn.Close()

	ch, err := rabbitConn.Channel()
	if err != nil {
		log.Fatal(err)
	}
	defer ch.Close()

	queueName := "client_info"
	msgs, err := ch.Consume(
		queueName, // queue
		"",        // consumer
		true,      // auto-ack
		false,     // exclusive
		false,     // no-local
		false,     // no-wait
		nil,       // args
	)
	if err != nil {
		log.Fatal(err)
	}

	for msg := range msgs {
		// Обработка сообщения
		handleMessage(msg.Body)
	}
}

func handleMessage(body []byte) {
	// Преобразуем содержимое сообщения в строку
	message := string(body)
	fmt.Println("Received message from RabbitMQ:", message)

	// Разбиваем сообщение на имя и email
	parts := strings.Split(message, ", ")
	if len(parts) != 2 {
		log.Println("Invalid message format:", message)
		return
	}

	name := strings.TrimPrefix(parts[0], "Имя: ")
	email := strings.TrimPrefix(parts[1], "Email: ")

	// Отправляем приветственное сообщение на почту клиента
	sendWelcomeEmail(name, email)
}
func sendWelcomeEmail(name, email string) {
	// Настройка клиента для отправки почты
	sender := "mymail.timoshenko@mail.ru"
	password := "d2UJAtv97NzydZML6KMx"
	host := "smtp.mail.ru"
	port := 465

	// Создание сообщения
	m := gomail.NewMessage()
	m.SetHeader("From", sender)
	m.SetHeader("To", email)
	m.SetHeader("Subject", "Добро пожаловать, "+name+"!")
	m.SetBody("text/html", "<b>Привет, "+name+"!</b><br>Спасибо за регистрацию.")

	// Создание клиента для отправки почты
	d := gomail.NewDialer(host, port, sender, password)
	d.TLSConfig = &tls.Config{InsecureSkipVerify: true}

	// Отправка сообщения
	retryAttempts := 3             // Количество попыток
	retryDelay := time.Second * 10 // Задержка между попытками

	// Повторная отправка сообщения в случае ошибки
	for attempt := 1; attempt <= retryAttempts; attempt++ {
		// Отправка сообщения
		if err := d.DialAndSend(m); err != nil {
			// Обработка ошибки
			log.Printf("Ошибка при отправке письма на адрес %s в попытке %d: %v", email, attempt, err)
			if attempt < retryAttempts {
				time.Sleep(retryDelay)
				continue // Повторная попытка отправки
			}
			log.Fatalf("Достигнуто максимальное количество попыток отправки письма на адрес %s", email)
		}

		fmt.Printf("Приветственное письмо отправлено на адрес %s\n", email)
		break // Выход из цикла при успешной отправке.
	}
}
