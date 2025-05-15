package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	tele "gopkg.in/telebot.v3"
)

// Subscriber represents a subscriber with ID and Username.
type Subscriber struct {
	ID       int64  `json:"id"`
	Username string `json:"username"` // May be empty if user has no username
}

// Tag represents a tag with its creator, description, and subscribers.
type Tag struct {
	Name        string       `json:"name"`
	CreatorID   int64        `json:"creator_id"`
	CreatorName string       `json:"creator_name"`
	Description string       `json:"description"`
	Subscribers []Subscriber `json:"subscribers"`
	CreatedAt   time.Time    `json:"created_at"`
}

// Data holds all tags.
type Data struct {
	Tags []Tag `json:"tags"`
}

// Bot state and data.
var (
	data     Data
	dataFile = "tags.json"
)

// loadData loads tags from JSON file and handles migration from old format.
func loadData() error {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN not set")
	}

	// If file doesn't exist, initialize empty data
	if _, err := os.Stat(dataFile); os.IsNotExist(err) {
		data = Data{Tags: []Tag{}}
		return saveData()
	}

	// Read file
	file, err := ioutil.ReadFile(dataFile)
	if err != nil {
		return err
	}

	// Try to unmarshal into new format
	err = json.Unmarshal(file, &data)
	if err != nil {
		// If unmarshal fails, try to load old format
		type OldTag struct {
			Name        string    `json:"name"`
			CreatorID   int64     `json:"creator_id"`
			CreatorName string    `json:"creator_name"`
			Description string    `json:"description"`
			Subscribers []int64   `json:"subscribers"`
			CreatedAt   time.Time `json:"created_at"`
		}
		type OldData struct {
			Tags []OldTag `json:"tags"`
		}

		var oldData OldData
		if err := json.Unmarshal(file, &oldData); err != nil {
			return fmt.Errorf("failed to unmarshal old and new data formats: %v", err)
		}

		// Convert old format to new format
		data.Tags = make([]Tag, len(oldData.Tags))
		for i, oldTag := range oldData.Tags {
			newSubscribers := make([]Subscriber, len(oldTag.Subscribers))
			for j, subID := range oldTag.Subscribers {
				newSubscribers[j] = Subscriber{
					ID:       subID,
					Username: fmt.Sprintf("User%d", subID), // Placeholder username
				}
			}
			data.Tags[i] = Tag{
				Name:        oldTag.Name,
				CreatorID:   oldTag.CreatorID,
				CreatorName: oldTag.CreatorName,
				Description: oldTag.Description,
				Subscribers: newSubscribers,
				CreatedAt:   oldTag.CreatedAt,
			}
		}

		// Save migrated data
		if err := saveData(); err != nil {
			return fmt.Errorf("failed to save migrated data: %v", err)
		}
		log.Println("Successfully migrated old data format to new format")
	}

	return nil
}

// saveData saves tags to JSON file.
func saveData() error {
	file, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(dataFile, file, 0644)
}

// findTag searches for a tag by name (case-insensitive).
func findTag(name string) *Tag {
	name = strings.ToLower(name)
	for i, tag := range data.Tags {
		if strings.ToLower(tag.Name) == name {
			return &data.Tags[i]
		}
	}
	return nil
}

// cleanEmptyTags removes tags with no subscribers.
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
	// Load environment variable for bot token
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	if token == "" {
		log.Fatal("TELEGRAM_BOT_TOKEN not set")
	}

	// Initialize bot
	bot, err := tele.NewBot(tele.Settings{
		Token:  token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		log.Fatal(err)
	}

	// Load data
	if err := loadData(); err != nil {
		log.Fatal(err)
	}

	// Handle /start
	bot.Handle("/start", func(c tele.Context) error {
		return c.Send("Привет! Я бот для управления тегами. Используй:\n" +
			"/ct <тег> [описание] — создать тег\n" +
			"/st <тег> — подписаться на тег\n" +
			"/dt <тег> — удалить тег\n" +
			"/lt — список всех тегов\n" +
			"/mt — твои теги\n" +
			"/stats — статистика тегов\n" +
			"Тег упоминается через #тег")
	})

	// Handle /ct (create tag)
	bot.Handle("/ct", func(c tele.Context) error {
		args := strings.Fields(c.Text())[1:]
		if len(args) == 0 {
			return c.Send("Укажи название тега: /ct <тег> [описание]")
		}

		tagName := args[0]
		if len(tagName) > 50 {
			return c.Send("Название тега слишком длинное (макс. 50 символов)")
		}

		// Check if tag already exists
		if findTag(tagName) != nil {
			return c.Send("Тег уже существует!")
		}

		// Check user tag limit
		userTags := 0
		for _, tag := range data.Tags {
			if tag.CreatorID == c.Sender().ID {
				userTags++
			}
		}
		if userTags >= 10 {
			return c.Send("Ты достиг лимита в 10 тегов!")
		}

		// Get description
		description := ""
		if len(args) > 1 {
			description = strings.Join(args[1:], " ")
			if len(description) > 100 {
				return c.Send("Описание слишком длинное (макс. 100 символов)")
			}
		}

		// Create tag
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

		return c.Send(fmt.Sprintf("Всем привет! @%s создал тег #%s\nОписание: %s",
			c.Sender().Username, tagName, description))
	})

	// Handle /st (subscribe to tag)
	bot.Handle("/st", func(c tele.Context) error {
		args := strings.Fields(c.Text())[1:]
		if len(args) == 0 {
			return c.Send("Укажи название тега: /st <тег>")
		}

		tag := findTag(args[0])
		if tag == nil {
			return c.Send("Тег не найден!")
		}

		// Check if already subscribed
		for _, sub := range tag.Subscribers {
			if sub.ID == c.Sender().ID {
				return c.Send("Ты уже подписан на этот тег!")
			}
		}

		// Subscribe
		username := c.Sender().Username
		if username == "" {
			username = fmt.Sprintf("User%d", c.Sender().ID) // Fallback if no username
		}
		tag.Subscribers = append(tag.Subscribers, Subscriber{
			ID:       c.Sender().ID,
			Username: username,
		})
		saveData()
		return c.Send(fmt.Sprintf("Ты подписался на #%s!", tag.Name))
	})

	// Handle /dt (delete tag)
	bot.Handle("/dt", func(c tele.Context) error {
		args := strings.Fields(c.Text())[1:]
		if len(args) == 0 {
			return c.Send("Укажи название тега: /dt <тег>")
		}

		tag := findTag(args[0])
		if tag == nil {
			return c.Send("Тег не найден!")
		}

		// Check if user is creator or admin
		isAdmin := false
		if chat := c.Chat(); chat != nil {
			admins, err := bot.AdminsOf(chat)
			if err == nil {
				for _, admin := range admins {
					if admin.User.ID == c.Sender().ID {
						isAdmin = true
						break
					}
				}
			}
		}
		if tag.CreatorID != c.Sender().ID && !isAdmin {
			return c.Send("Только создатель тега или админ могут его удалить!")
		}

		// Remove tag
		newTags := []Tag{}
		for _, t := range data.Tags {
			if strings.ToLower(t.Name) != strings.ToLower(tag.Name) {
				newTags = append(newTags, t)
			}
		}
		data.Tags = newTags
		saveData()
		return c.Send(fmt.Sprintf("Тег #%s удален!", tag.Name))
	})

	// Handle /lt (list all tags)
	bot.Handle("/lt", func(c tele.Context) error {
		cleanEmptyTags()
		if len(data.Tags) == 0 {
			return c.Send("Тегов пока нет!")
		}

		var response strings.Builder
		response.WriteString("Список всех тегов:\n")
		for _, tag := range data.Tags {
			response.WriteString(fmt.Sprintf("#%s (%d подписчиков): %s\n",
				tag.Name, len(tag.Subscribers), tag.Description))
		}
		return c.Send(response.String())
	})

	// Handle /mt (my tags)
	bot.Handle("/mt", func(c tele.Context) error {
		var response strings.Builder
		response.WriteString("Твои теги:\n")
		found := false
		for _, tag := range data.Tags {
			for _, sub := range tag.Subscribers {
				if sub.ID == c.Sender().ID {
					response.WriteString(fmt.Sprintf("#%s: %s\n", tag.Name, tag.Description))
					found = true
				}
			}
		}
		if !found {
			response.WriteString("Ты не подписан ни на один тег!")
		}
		return c.Send(response.String())
	})

	// Handle /stats
	bot.Handle("/stats", func(c tele.Context) error {
		cleanEmptyTags()
		if len(data.Tags) == 0 {
			return c.Send("Тегов пока нет!")
		}

		var response strings.Builder
		response.WriteString("Статистика тегов:\n")
		for _, tag := range data.Tags {
			response.WriteString(fmt.Sprintf("#%s: %d подписчиков\n",
				tag.Name, len(tag.Subscribers)))
		}
		return c.Send(response.String())
	})

	// Handle tag mentions (#tag)
	bot.Handle(tele.OnText, func(c tele.Context) error {
		text := c.Text()
		if !strings.Contains(text, "#") {
			return nil
		}

		words := strings.Fields(text)
		var mentions []string
		for _, word := range words {
			if strings.HasPrefix(word, "#") {
				tagName := strings.TrimPrefix(word, "#")
				tag := findTag(tagName)
				if tag != nil {
					log.Printf("Found tag: %s", tagName)
					log.Printf("Tag %s has %d subscribers", tagName, len(tag.Subscribers))
					for _, sub := range tag.Subscribers {
						if sub.Username != "" && sub.Username != fmt.Sprintf("User%d", sub.ID) {
							mentions = append(mentions, fmt.Sprintf("@%s", sub.Username))
						}
					}
				}
			}
		}

		if len(mentions) > 0 {
			log.Printf("Sending mentions: %v", mentions)
			return c.Send(strings.Join(mentions, " ") + "\nТег упомянут!")
		}
		return nil
	})

	// Start bot
	log.Println("Bot started...")
	bot.Start()
}
