package main

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// model represents the application state for the TUI.
type model struct {
	state              appState
	userLang           string
	targetLang         string
	input              string
	originalSentence   string
	translation        string
	wordAnalysis       []wordInfo
	err                error
	cursor             int
	selectedLang       int
	langs              []language
	langFilter         string
	filteredLangs      []language
	showUserLangMenu   bool
	showTargetLangMenu bool
}

// appState represents the current state of the application.
type appState int

const (
	stateSelectUserLang appState = iota
	stateSelectTargetLang
	stateInputSentence
	stateShowResults
)

// language represents a language with its code and display name.
type language struct {
	code string
	name string
}

// wordInfo represents a single word analysis result.
type wordInfo struct {
	WordInTargetLang       string `json:"word_in_target_lang"`
	GrammaticalExplanation string `json:"grammatical_explanation"`
}

// Languages that can be selected as "known well" for my personal convenience
var knownLanguages = []language{
	{"de", "German"},
	{"sv", "Swedish"},
	{"en", "English"},
	{"es", "Spanish"},
}

// All languages available for learning (includes all current languages + German)
var allTargetLanguages = []language{
	{"sr", "Serbian"},
	{"es", "Spanish"},
	{"fr", "French"},
	{"it", "Italian"},
	{"pt", "Portuguese"},
	{"en", "English"},
	{"sv", "Swedish"},
	{"de", "German"},
}

var (
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("51")).
			Bold(true).
			Padding(1, 2)

	selectedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("39")).
			Bold(true)

	normalStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("231"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196")).
			Bold(true)

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("46"))

	labelStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("87")).
			Bold(true)

	valueStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("231"))
)

func initialModel() model {
	return model{
		state:            stateSelectUserLang,
		langs:            knownLanguages,
		filteredLangs:    knownLanguages,
		showUserLangMenu: true,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.state == stateShowResults {
				m.state = stateInputSentence
				m.translation = ""
				m.wordAnalysis = nil
				return m, nil
			}
			return m, tea.Quit

		case "esc":
			if m.state == stateSelectUserLang {
				if m.showUserLangMenu || m.showTargetLangMenu {
					return m, tea.Quit
				}
				m.state = stateSelectUserLang
				m.showUserLangMenu = true
				m.showTargetLangMenu = false
				m.selectedLang = 0
				m.langFilter = ""
				m.langs = knownLanguages
				m.filteredLangs = m.langs
				return m, nil
			}

			if m.state == stateSelectTargetLang {
				m.state = stateSelectUserLang
				m.showTargetLangMenu = false
				m.showUserLangMenu = true
				m.selectedLang = 0
				m.langFilter = ""
				m.langs = knownLanguages
				m.filteredLangs = m.langs
				return m, nil
			}

			if m.state == stateInputSentence {
				m.state = stateSelectTargetLang
				m.showTargetLangMenu = true
				m.showUserLangMenu = false
				m.input = ""
				m.selectedLang = 0
				m.langFilter = ""
				m.langs = getAvailableTargetLanguages(m.userLang)
				m.filteredLangs = m.langs
				return m, nil
			}

		case "enter":
			if m.state == stateSelectUserLang {
				if len(m.filteredLangs) > 0 {
					m.userLang = m.filteredLangs[m.selectedLang].code
					m.state = stateSelectTargetLang
					m.showUserLangMenu = false
					m.showTargetLangMenu = true
					m.selectedLang = 0
					m.langFilter = ""
					// Set available languages to all target languages, excluding user's language
					m.langs = getAvailableTargetLanguages(m.userLang)
					m.filteredLangs = m.langs
				}
				return m, nil
			}
			if m.state == stateSelectTargetLang {
				if len(m.filteredLangs) > 0 {
					m.targetLang = m.filteredLangs[m.selectedLang].code
					m.state = stateInputSentence
					m.showTargetLangMenu = false
				}
				return m, nil
			}
			if m.state == stateInputSentence && m.input != "" {
				return m, translateSentence(m.userLang, m.targetLang, m.input)
			}

		case "up":
			if m.state == stateSelectUserLang || m.state == stateSelectTargetLang {
				if m.selectedLang > 0 {
					m.selectedLang--
				}
				return m, nil
			}

		case "down":
			if m.state == stateSelectUserLang || m.state == stateSelectTargetLang {
				if m.selectedLang < len(m.filteredLangs)-1 {
					m.selectedLang++
				}
				return m, nil
			}

		case "backspace":
			if m.state == stateSelectUserLang || m.state == stateSelectTargetLang {
				if len(m.langFilter) > 0 {
					m.langFilter = m.langFilter[:len(m.langFilter)-1]
					m.filterLanguages()
					if m.selectedLang >= len(m.filteredLangs) {
						m.selectedLang = len(m.filteredLangs) - 1
						if m.selectedLang < 0 {
							m.selectedLang = 0
						}
					}
				}
				return m, nil
			}
			if m.state == stateInputSentence {
				if len(m.input) > 0 {
					m.input = m.input[:len(m.input)-1]
				}
				return m, nil
			}

		default:
			if m.state == stateSelectUserLang || m.state == stateSelectTargetLang {
				if len(msg.String()) == 1 {
					m.langFilter += msg.String()
					m.filterLanguages()
					m.selectedLang = 0
					return m, nil
				}
			}
			if m.state == stateInputSentence {
				m.input += msg.String()
				return m, nil
			}
		}

	case translationResult:
		if msg.err != nil {
			m.err = msg.err
			return m, nil
		}
		m.translation = msg.translation
		m.originalSentence = msg.originalSentence
		m.wordAnalysis = msg.wordAnalysis
		m.state = stateShowResults
		m.input = ""
		m.err = nil
		return m, nil
	}

	return m, nil
}

func (m *model) filterLanguages() {
	if m.langFilter == "" {
		m.filteredLangs = m.langs
		return
	}
	filter := strings.ToLower(m.langFilter)
	m.filteredLangs = []language{}
	for _, lang := range m.langs {
		if strings.Contains(strings.ToLower(lang.name), filter) ||
			strings.Contains(strings.ToLower(lang.code), filter) {
			m.filteredLangs = append(m.filteredLangs, lang)
		}
	}
}

func (m model) View() string {
	var s strings.Builder

	switch m.state {
	case stateSelectUserLang:
		s.WriteString(titleStyle.Render("Select A Language You Know Well:"))
		s.WriteString("\n\n")
		if m.langFilter != "" {
			s.WriteString(fmt.Sprintf("Filter: %s\n\n", m.langFilter))
		}
		for i, lang := range m.filteredLangs {
			if i == m.selectedLang {
				s.WriteString(selectedStyle.Render(fmt.Sprintf("> %s (%s)", lang.name, lang.code)))
			} else {
				s.WriteString(normalStyle.Render(fmt.Sprintf("  %s (%s)", lang.name, lang.code)))
			}
			s.WriteString("\n")
		}
		s.WriteString("\n")
		s.WriteString(normalStyle.Render("↑/↓: Navigate | Enter: Select | Esc: Quit | Type to filter"))

	case stateSelectTargetLang:
		s.WriteString(titleStyle.Render("Select The Language You Want To Learn:"))
		s.WriteString("\n\n")
		s.WriteString(fmt.Sprintf("From: %s\n\n", m.getLangName(m.userLang)))
		if m.langFilter != "" {
			s.WriteString(fmt.Sprintf("Filter: %s\n\n", m.langFilter))
		}
		for i, lang := range m.filteredLangs {
			if i == m.selectedLang {
				s.WriteString(selectedStyle.Render(fmt.Sprintf("> %s (%s)", lang.name, lang.code)))
			} else {
				s.WriteString(normalStyle.Render(fmt.Sprintf("  %s (%s)", lang.name, lang.code)))
			}
			s.WriteString("\n")
		}
		s.WriteString("\n")
		s.WriteString(normalStyle.Render("↑/↓: Navigate | Enter: Select | Esc: Back | Type to filter"))

	case stateInputSentence:
		s.WriteString(titleStyle.Render("Enter Sentence in Either Language:"))
		s.WriteString("\n\n")
		s.WriteString(fmt.Sprintf("%s ↔ %s\n\n", m.getLangName(m.userLang), m.getLangName(m.targetLang)))
		s.WriteString(fmt.Sprintf("Sentence: %s", m.input))
		if m.cursor%2 == 0 {
			s.WriteString("█")
		}
		s.WriteString("\n\n")
		if m.err != nil {
			s.WriteString(errorStyle.Render(fmt.Sprintf("Error: %v\n\n", m.err)))
		}
		s.WriteString(normalStyle.Render("Enter: Translate | Esc: Back | Ctrl+C: Quit"))

	case stateShowResults:
		s.WriteString(titleStyle.Render("Translation Results"))
		s.WriteString("\n\n")
		s.WriteString(fmt.Sprintf("%s ↔ %s\n\n", m.getLangName(m.userLang), m.getLangName(m.targetLang)))
		s.WriteString(labelStyle.Render("Original: "))
		s.WriteString(valueStyle.Render(m.originalSentence))
		s.WriteString("\n\n")
		s.WriteString(labelStyle.Render("Translation: "))
		s.WriteString(successStyle.Render(m.translation))
		s.WriteString("\n\n")

		if len(m.wordAnalysis) > 0 {
			s.WriteString(labelStyle.Render("Word-by-Word Analysis:\n"))
			s.WriteString("\n")
			for _, word := range m.wordAnalysis {
				s.WriteString(fmt.Sprintf("  %s", valueStyle.Render(word.WordInTargetLang)))
				if word.GrammaticalExplanation != "" {
					s.WriteString(" - ")
					s.WriteString(normalStyle.Render(word.GrammaticalExplanation))
				}
				s.WriteString("\n")
			}
		}
		s.WriteString("\n")
		s.WriteString(normalStyle.Render("Press 'q' or Ctrl+C to translate another | Esc: Back"))

	default:
		s.WriteString("Unknown state")
	}

	return s.String()
}

func (m model) getLangName(code string) string {
	// Check in all possible languages, not just current langs
	allLangs := append(knownLanguages, allTargetLanguages...)
	for _, lang := range allLangs {
		if lang.code == code {
			return lang.name
		}
	}
	return code
}

// getAvailableTargetLanguages returns all target languages except the selected known language
func getAvailableTargetLanguages(userLang string) []language {
	available := []language{}
	for _, lang := range allTargetLanguages {
		if lang.code != userLang {
			available = append(available, lang)
		}
	}
	return available
}
