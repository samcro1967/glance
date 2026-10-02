package glance

import (
	"context"
	"fmt"
	"html"
	"html/template"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

var triviaWidgetTemplate = mustParseTemplate("trivia.html", "widget-base.html")

const openTriviaDBURL = "https://opentdb.com/api.php?amount=1&type=multiple"

type triviaWidget struct {
	widgetBase `yaml:",inline"`
	Question   triviaQuestion `yaml:"-"`
}
type triviaQuestion struct {
	Category, Difficulty, Question string
	Answers                        []triviaAnswer
}
type triviaAnswer struct {
	Text    string
	Correct bool
}
type openTriviaResponse struct {
	ResponseCode int `json:"response_code"`
	Results      []struct {
		Category         string   `json:"category"`
		Difficulty       string   `json:"difficulty"`
		Question         string   `json:"question"`
		CorrectAnswer    string   `json:"correct_answer"`
		IncorrectAnswers []string `json:"incorrect_answers"`
	} `json:"results"`
}

func (w *triviaWidget) initialize() error {
	w.withTitle("Trivia").withCacheDuration(24 * time.Hour)
	return nil
}
func (w *triviaWidget) update(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dailyDiscoveryProviderURL("/daily-discovery/trivia", openTriviaDBURL), nil)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(err)
		return
	}
	req.Header.Set("User-Agent", glanceUserAgentString)
	response, err := decodeJsonFromRequest[openTriviaResponse](defaultHTTPClient, req)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(fmt.Errorf("fetching trivia: %w", err))
		return
	}
	question, err := parseTriviaQuestion(response)
	if err != nil {
		w.canContinueUpdateAfterHandlingErr(err)
		return
	}
	if !w.canContinueUpdateAfterHandlingErr(nil) {
		return
	}
	w.Question = question
}
func (w *triviaWidget) Render() template.HTML { return w.renderTemplate(w, triviaWidgetTemplate) }
func parseTriviaQuestion(r openTriviaResponse) (triviaQuestion, error) {
	if r.ResponseCode != 0 || len(r.Results) != 1 {
		return triviaQuestion{}, fmt.Errorf("Open Trivia DB returned response code %d with %d questions", r.ResponseCode, len(r.Results))
	}
	src := r.Results[0]
	if strings.TrimSpace(src.Question) == "" || strings.TrimSpace(src.CorrectAnswer) == "" || len(src.IncorrectAnswers) == 0 {
		return triviaQuestion{}, fmt.Errorf("Open Trivia DB returned an incomplete question")
	}
	answers := make([]triviaAnswer, 0, len(src.IncorrectAnswers)+1)
	answers = append(answers, triviaAnswer{Text: html.UnescapeString(src.CorrectAnswer), Correct: true})
	for _, answer := range src.IncorrectAnswers {
		if strings.TrimSpace(answer) != "" {
			answers = append(answers, triviaAnswer{Text: html.UnescapeString(answer)})
		}
	}
	rand.Shuffle(len(answers), func(i, j int) { answers[i], answers[j] = answers[j], answers[i] })
	difficulty := strings.TrimSpace(src.Difficulty)
	if difficulty != "" {
		difficulty = strings.ToUpper(difficulty[:1]) + difficulty[1:]
	}
	return triviaQuestion{Category: html.UnescapeString(src.Category), Difficulty: difficulty, Question: html.UnescapeString(src.Question), Answers: answers}, nil
}
