# Используем образ Golang
FROM golang:latest

# Устанавливаем переменную окружения для пути к проекту внутри контейнера
ENV APP_HOME /app
WORKDIR $APP_HOME

# Копируем файлы проекта в контейнер
COPY main .

# Собираем исполняемый файл
RUN go build -o main .

# Команда для запуска приложения
CMD ["./main"]