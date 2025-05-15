package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/joho/godotenv"
	tele "gopkg.in/telebot.v3"
)

type Subscriber struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

type Tag struct {
	Name        string       `json:"name"`
	CreatorID   int64        `json:"creator_id"`
	CreatorName string       `json:"creator_name"`
	Description string       `json:"description"`
	Subscribers []Subscriber `json:"subscribers"`
	CreatedAt   time.Time    `json:"created_at"`
}

type Data struct {
	Tags []Tag `json:"tags"`
}

var (
	data     Data
	dataFile = "tags.json"
	funnyPhrases = []string{
		"Ау! Китайские сыновья солнца, вас тут пингуют.",
		"Просыпайтесь, воины тега #%s!",
		"Снова вы, #%s? Ну давайте...",
		"Собрание #%s начинается. Кто опоздает — тот кодит в пятницу вечером!",
		"🔔 Призыв по тегу #%s! Сбор у обелиска.",
	}
)

func loadData() error {
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		data = Data{Tags: []Tag{}}
		return saveData()
	}
	file, err := ioutil.ReadFile(dataFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(file, &data)
}

func saveData() error {
	file, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(dataFile, file, 0644)
}

func findTag(name string) *Tag {
	name = strings.ToLower(name)
	for i, tag := range data.Tags {
		if strings.ToLower(tag.Name) == name {
			return &data.Tags[i]
		}
	}
	return nil
}

func cleanEmptyTags() {
	newTags := []Tag{}
	for _, tag := range data.Tags {
		if len(tag.Subscribers) > 0 {
			newTags = append(newTags, tag)
		}
	}
	data.Tags = newTags
	saveData()
}

func main() {
	_ = godotenv.Load()
	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN not set")
	}

	bot, err := tele.NewBot(tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		log.Fatal(err)
	}

	if err := loadData(); err != nil {
		log.Fatal(err)
	}

	bot.Handle("/start", func(c tele.Context) error {
		return c.Send("👋 Привет! Я бот для тегов. Команды:\n\n"+
			"/ct <тег> [описание] — создать тег\n"+
			"/st <тег> — подписаться\n"+
			"/dt <тег> — удалить\n"+
			"/lt — все теги\n"+
			"/mt — мои теги\n"+
			"/stats — статистика\n\nТег упоминается через #тег")
	})

	bot.Handle("/ct", func(c tele.Context) error {
		args := strings.Fields(c.Text())[1:]
		if len(args) == 0 {
			return c.Send("❗ Укажи название тега: /ct <тег> [описание]")
		}
		tagName := args[0]
		if findTag(tagName) != nil {
			return c.Send("⚠️ Такой тег уже существует!")
		}
		description := ""
		if len(args) > 1 {
			description = strings.Join(args[1:], " ")
		}
		tag := Tag{
			Name:        tagName,
			CreatorID:   c.Sender().ID,
			CreatorName: c.Sender().Username,
			Description: description,
			Subscribers: []Subscriber{},
			CreatedAt:   time.Now(),
		}
		data.Tags = append(data.Tags, tag)
		saveData()
		return c.Send(fmt.Sprintf("🌟 *Новый тег создан!\n👤 Создатель:* @%s\n🏷️ *Тег:* `#%s`\n📜 *Описание:* %s",
			c.Sender().Username, tagName, description), tele.ModeMarkdown)
	})

	bot.Handle("/st", func(c tele.Context) error {
		args := strings.Fields(c.Text())[1:]
		if len(args) == 0 {
			return c.Send("❗ Укажи тег: /st <тег>")
		}
		tag := findTag(args[0])
		if tag == nil {
			return c.Send("⛔ Тег не найден!")
		}
		for _, sub := range tag.Subscribers {
			if sub.ID == c.Sender().ID {
				return c.Send("✅ Ты уже подписан!")
			}
		}
		username := c.Sender().Username
		if username == "" {
			username = fmt.Sprintf("User%d", c.Sender().ID)
		}
		tag.Subscribers = append(tag.Subscribers, Subscriber{ID: c.Sender().ID, Username: username})
		saveData()
		return c.Send(fmt.Sprintf("📬 Подписка на `#%s` оформлена!", tag.Name), tele.ModeMarkdown)
	})

	bot.Handle("/dt", func(c tele.Context) error {
		args := strings.Fields(c.Text())[1:]
		if len(args) == 0 {
			return c.Send("❗ Укажи тег: /dt <тег>")
		}
		tag := findTag(args[0])
		if tag == nil {
			return c.Send("⛔ Тег не найден!")
		}
		if tag.CreatorID != c.Sender().ID {
			return c.Send("🚫 Только создатель может удалить тег!")
		}
		newTags := []Tag{}
		for _, t := range data.Tags {
			if strings.ToLower(t.Name) != strings.ToLower(tag.Name) {
				newTags = append(newTags, t)
			}
		}
		data.Tags = newTags
		saveData()
		return c.Send(fmt.Sprintf("🗑️ Тег `#%s` удалён!", tag.Name), tele.ModeMarkdown)
	})

	bot.Handle("/lt", func(c tele.Context) error {
		cleanEmptyTags()
		if len(data.Tags) == 0 {
			return c.Send("📭 Пока тегов нет!")
		}
		var b strings.Builder
		b.WriteString("📚 *Список тегов:*\n")
		for _, tag := range data.Tags {
			b.WriteString(fmt.Sprintf("`#%s` (%d): %s\n", tag.Name, len(tag.Subscribers), tag.Description))
		}
		return c.Send(b.String(), tele.ModeMarkdown)
	})

	bot.Handle("/mt", func(c tele.Context) error {
		var b strings.Builder
		b.WriteString("📌 *Твои теги:*\n")
		found := false
		for _, tag := range data.Tags {
			for _, sub := range tag.Subscribers {
				if sub.ID == c.Sender().ID {
					b.WriteString(fmt.Sprintf("`#%s` — %s\n", tag.Name, tag.Description))
					found = true
				}
			}
		}
		if !found {
			b.WriteString("_Ты не подписан ни на один тег._")
		}
		return c.Send(b.String(), tele.ModeMarkdown)
	})

	bot.Handle("/stats", func(c tele.Context) error {
		cleanEmptyTags()
		var b strings.Builder
		b.WriteString("📊 *Статистика:*\n")
		for _, tag := range data.Tags {
			b.WriteString(fmt.Sprintf("`#%s` — %d подписчиков\n", tag.Name, len(tag.Subscribers)))
		}
		return c.Send(b.String(), tele.ModeMarkdown)
	})

	bot.Handle(tele.OnText, func(c tele.Context) error {
		text := c.Text()
		re := regexp.MustCompile(`#([A-Za-zА-Яа-я0-9_]+)`)
		matches := re.FindAllStringSubmatch(text, -1)
		var responses []string
		for _, match := range matches {
			tagName := match[1]
			tag := findTag(tagName)
			if tag == nil {
				continue
			}
			var mentions []string
			for _, sub := range tag.Subscribers {
				if sub.Username != "" && sub.Username != fmt.Sprintf("User%d", sub.ID) {
					mentions = append(mentions, fmt.Sprintf("@%s", sub.Username))
				}
			}
			if len(mentions) > 0 {
				phrase := fmt.Sprintf(funnyPhrases[rand.Intn(len(funnyPhrases))], tagName)
				responses = append(responses, fmt.Sprintf("%s\n%s", strings.Join(mentions, " "), phrase))
			}
		}
		if len(responses) > 0 {
			return c.Send(strings.Join(responses, "\n\n"))
		}
		return nil
	})

	log.Println("🤖 Бот запущен...")
	bot.Start()
}
