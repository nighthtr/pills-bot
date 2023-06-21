package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/joho/godotenv"
)

type SearchMedicineRequest struct {
	ApiKey       string `json:"api_key"`
	State        string `json:"state"`
	HoumeCountry int    `json:"home_country"`
	Query        string `json:"query"`
}

type SearchMedicineResponse struct {
	Medicines []Medicine `json:"medicines"`
}

type SearchAnalogRequest struct {
	ApiKey        string `json:"api_key"`
	State         string `json:"state"`
	HoumeCountry  int    `json:"home_country"`
	TargetCountry int    `json:"target_country"`
	Language      string `json:"language"`
	Medicine      int    `json:"medicine"`
}

type SearchAnalogResponse struct {
	MedicineInfo MedicineInfo `json:"medicine_info"`
	HomeCountry  MedicineInfo `json:"home_country"`
	Analogs      []Analog     `json:"medicine_analogs"`
}

type Medicine struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Components string `json:"components"`
	Slug       string `json:"slug"`
	IsPopular  int    `json:"ispopular"`
}

type MedicineInfo struct {
	MedicineID   string `json:"medicine_id"`
	MedicineName string `json:"medicine_name"`
	MedicineSlug string `json:"medicine_slug"`
	DateRevision string `json:"date_revision"`
}

type Analog struct {
	AnalogID        string `json:"analog_id"`
	AnalogName      string `json:"analog_name"`
	AnalogSlug      string `json:"analog_slug"`
	ComponentsMatch int    `json:"components_match"`
	ApplyingsMatch  int    `json:"applyings_match"`
	TreatmentsMatch int    `json:"treatments_match"`
	Percentage      int    `json:"percentage"`
}

var (
	ApiUrl          string = "https://api.pillintrip.com/search"
	ApiKey          string
	HoumeCountryID  int
	TargetCountryID int
	err             error
)

func init() {
	err := godotenv.Load(".env")

	if err != nil {
		log.Fatal("Error loading .env file")
	}
}

func main() {
	BotToken := os.Getenv("BOT_TOKEN")
	if len(BotToken) == 0 {
		log.Fatal("Не указан токен телеграм бота")
		os.Exit(2)
	}

	ApiKey = os.Getenv("API_KEY")
	if len(ApiKey) == 0 {
		log.Fatal("Не указан ключ API")
		os.Exit(2)
	}

	HoumeCountryID, err = strconv.Atoi(os.Getenv("HOME_COUNTRY_ID"))
	if err != nil {
		HoumeCountryID = 0
	}
	if HoumeCountryID == 0 {
		log.Fatal("Не указана домашняя страна")
		os.Exit(2)
	}

	TargetCountryID, err = strconv.Atoi(os.Getenv("TARGET_COUNTRY_ID"))
	if err != nil {
		TargetCountryID = 0
	}
	if TargetCountryID == 0 {
		log.Fatal("Не указана страна поиска")
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	opts := []bot.Option{
		bot.WithDefaultHandler(searchMedicineHandler),
		bot.WithCallbackQueryDataHandler("search_analog", bot.MatchTypePrefix, searcheAnalogHandler),
		bot.WithCallbackQueryDataHandler("show_medicine", bot.MatchTypePrefix, showMedicineHandler),
	}

	b, err := bot.New(BotToken, opts...)
	if err != nil {
		log.Fatal(err)
		os.Exit(2)
	}

	b.RegisterHandler(bot.HandlerTypeMessageText, "/start", bot.MatchTypeExact, startHandler)

	b.Start(ctx)
}

func startHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Привет. Я помогу вам найти аналоги лекарств в Таиланде. Для поиска введите название лекарства.",
	})
}

func searchMedicineHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	if update.Message == nil {
		return
	}

	medicines, err := searchMedicines(update.Message.Text)
	if err != nil || len(medicines) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.Message.Chat.ID,
			Text:   "Мне не удалось ничего найти.",
		})
		return
	}

	buttons := [][]models.InlineKeyboardButton{}
	for index, medicine := range medicines {
		if index == 10 {
			break
		}
		buttons = append(buttons, []models.InlineKeyboardButton{
			{
				Text:         medicine.Name,
				CallbackData: "search_analog:" + medicine.ID,
			},
		})
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.Message.Chat.ID,
		Text:   "Вот что я нашел. Выберите лекарство, для которого нужно найти аналоги.",
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: buttons,
		},
	})
}

func searcheAnalogHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		ShowAlert:       false,
	})

	medicineID, _ := strconv.Atoi(strings.Split(update.CallbackQuery.Data, ":")[1])

	analogs, medicineInfo, err := searchAnalogs(medicineID)
	if err != nil || len(analogs) == 0 {
		b.SendMessage(ctx, &bot.SendMessageParams{
			ChatID: update.CallbackQuery.Message.Chat.ID,
			Text:   fmt.Sprintf("Мне не удалось найти аналоги для \"%s\".", medicineInfo.MedicineName),
		})
		return
	}

	buttons := [][]models.InlineKeyboardButton{}
	for index, analog := range analogs {
		if index == 10 {
			break
		}
		buttons = append(buttons, []models.InlineKeyboardButton{
			{
				Text: analog.AnalogName + " (" + strconv.Itoa(analog.Percentage) + "%)",
				// CallbackData: "show_medicine:" + analog.AnalogID,
				URL: "https://pillintrip.com/ru/medicine/" + analog.AnalogSlug,
			},
		})
	}

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.CallbackQuery.Message.Chat.ID,
		Text:   fmt.Sprintf("Вот аналоги для \"%s\":", medicineInfo.MedicineName),
		ReplyMarkup: &models.InlineKeyboardMarkup{
			InlineKeyboard: buttons,
		},
	})
}

func showMedicineHandler(ctx context.Context, b *bot.Bot, update *models.Update) {
	b.AnswerCallbackQuery(ctx, &bot.AnswerCallbackQueryParams{
		CallbackQueryID: update.CallbackQuery.ID,
		ShowAlert:       false,
	})

	medicineID, _ := strconv.Atoi(strings.Split(update.CallbackQuery.Data, ":")[1])

	b.SendMessage(ctx, &bot.SendMessageParams{
		ChatID: update.CallbackQuery.Message.Chat.ID,
		Text:   fmt.Sprintf("Тут инфа по ценам для MedicineId=%d", medicineID),
	})
}

func searchMedicines(query string) ([]Medicine, error) {
	searchMedicineRequest := SearchMedicineRequest{
		ApiKey:       ApiKey,
		State:        "main_search",
		HoumeCountry: HoumeCountryID,
		Query:        query,
	}

	log.Printf("Поиск лекарств: %s\n", query)

	body, err := json.Marshal(searchMedicineRequest)
	if err != nil {
		log.Println(err)
		return []Medicine{}, err
	}

	request, err := http.NewRequest("POST", ApiUrl, bytes.NewBuffer(body))
	if err != nil {
		log.Println(err)
		return []Medicine{}, err
	}

	request.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		log.Println(err)
		return []Medicine{}, err
	}
	defer response.Body.Close()

	searchMedicineResponse := &SearchMedicineResponse{}
	err = json.NewDecoder(response.Body).Decode(searchMedicineResponse)
	if err != nil {
		log.Println(err)
		return []Medicine{}, err
	}

	return searchMedicineResponse.Medicines, nil
}

func searchAnalogs(medicineID int) ([]Analog, MedicineInfo, error) {
	searchAnalogRequest := SearchAnalogRequest{
		ApiKey:        ApiKey,
		State:         "main_search",
		HoumeCountry:  HoumeCountryID,
		TargetCountry: TargetCountryID,
		Language:      "ru",
		Medicine:      medicineID,
	}

	log.Printf("Поиск аналогов: %d\n", medicineID)

	body, err := json.Marshal(searchAnalogRequest)
	if err != nil {
		log.Println(err)
		return []Analog{}, MedicineInfo{}, err
	}

	request, err := http.NewRequest("POST", ApiUrl, bytes.NewBuffer(body))
	if err != nil {
		log.Println(err)
		return []Analog{}, MedicineInfo{}, err
	}

	request.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		log.Println(err)
		return []Analog{}, MedicineInfo{}, err
	}
	defer response.Body.Close()

	searchAnalogResponse := &SearchAnalogResponse{}
	err = json.NewDecoder(response.Body).Decode(searchAnalogResponse)
	if err != nil {
		log.Println(err)
		return searchAnalogResponse.Analogs, searchAnalogResponse.HomeCountry, err
	}

	return searchAnalogResponse.Analogs, searchAnalogResponse.MedicineInfo, nil
}
