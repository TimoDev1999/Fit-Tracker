package main

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"net/http"
)

type Pfc struct {
	id      string
	Protein float64 `json:"protein"`
	Fats    float64 `json:"fats"`
	Carbs   float64 `json:"carbs"`
	Date    string  `json:"date"`
	Name    string  `json:"name"`
}

func main() {
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
	router.Run() // listen and serve on 0.0.0.0:8080 (for windows "localhost:8080")
}
func mongoWriter(pfc Pfc) error {
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		panic(err)
	}

	usersCollection := client.Database("pfc").Collection("pfc")
	pfcWrite := bson.D{
		{"protein", pfc.Protein}, {"fats", pfc.Fats},
		{"carbs", pfc.Carbs}, {"name", pfc.Name}, {"date", pfc.Date}}
	result, err := usersCollection.InsertOne(context.TODO(), pfcWrite)
	if err != nil {
		panic(err)
	}
	fmt.Println(result.InsertedID)
	return nil
}

func getByDate(date string) ([]Pfc, error) {
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		panic(err)
	}
	usersCollection := client.Database("pfc").Collection("pfc")
	filter := bson.D{
		{"date", date},
	}
	var results []Pfc
	cursor, err := usersCollection.Find(context.TODO(), filter)
	// check for errors in the finding
	if err != nil {
		panic(err)
	}
	defer cursor.Close(context.TODO())

	for cursor.Next(context.TODO()) {
		var result Pfc
		err := cursor.Decode(&result)
		if err != nil {
			panic(err)
		}
		results = append(results, result)
	}
	if err := cursor.Err(); err != nil {
		panic(err)
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
