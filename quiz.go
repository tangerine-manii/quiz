package main

import (
	"fmt"
	"html/template"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// =====================
// 데이터 구조
// =====================

type Mode string

const (
	ModeNone     Mode = ""
	ModeSubject  Mode = "subject"
	ModeMultiple Mode = "multiple"
)

type QuizSession struct {
	Images  []string
	Current int
	Score   int
	Wrong   []WrongItem
	Done    bool
	Mode    Mode
}

type WrongItem struct {
	ImagePath string
	Answer    string
	UserInput string
}

var (
	session *QuizSession
	mu      sync.Mutex
)

// =====================
// 이미지 로드
// =====================

func loadImages(dir string) ([]string, error) {
	validExt := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	}
	var images []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if validExt[ext] {
			images = append(images, e.Name())
		}
	}
	rand.New(rand.NewSource(time.Now().UnixNano()))
	rand.Shuffle(len(images), func(i, j int) {
		images[i], images[j] = images[j], images[i]
	})

	// 30개만 자르기
	if len(images) > 30 {
		images = images[:30]
	}
	return images, nil
}

func getAnswer(filename string) string {
	return strings.TrimSuffix(filename, filepath.Ext(filename))
}

func checkAnswer(filename, userInput string) bool {
	return strings.EqualFold(strings.TrimSpace(userInput), strings.TrimSpace(getAnswer(filename)))
}

// 객관식 오답 보기 3개 생성
func makeChoices(images []string, currentIdx int) []string {
	answer := getAnswer(images[currentIdx])
	choicesSet := map[string]bool{answer: true}
	choices := []string{answer}

	pool := make([]int, 0, len(images))
	for i := range images {
		if i != currentIdx {
			pool = append(pool, i)
		}
	}
	rand.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })

	for _, idx := range pool {
		name := getAnswer(images[idx])
		if !choicesSet[name] {
			choicesSet[name] = true
			choices = append(choices, name)
		}
		if len(choices) == 4 {
			break
		}
	}

	rand.Shuffle(len(choices), func(i, j int) { choices[i], choices[j] = choices[j], choices[i] })
	return choices
}

// =====================
// 템플릿
// =====================

var selectTmpl = template.Must(template.New("select").Parse(`
<!DOCTYPE html>
<html lang="ko">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>이미지 퀴즈</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: 'Segoe UI', sans-serif;
    background: #f0f4f8; min-height: 100vh;
    display: flex; flex-direction: column;
    align-items: center; justify-content: center; padding: 20px;
  }
  .card {
    background: white; border-radius: 20px;
    box-shadow: 0 4px 24px rgba(0,0,0,0.12);
    padding: 48px 40px; max-width: 440px; width: 100%;
    text-align: center;
  }
  h1 { font-size: 2rem; color: #2d3748; margin-bottom: 8px; }
  p { color: #718096; margin-bottom: 40px; font-size: 1rem; }
  .btn {
    display: block; width: 100%; padding: 18px;
    border-radius: 14px; font-size: 1.1rem; font-weight: 700;
    text-decoration: none; margin-bottom: 14px;
    cursor: pointer; border: none; transition: opacity 0.2s;
  }
  .btn:hover { opacity: 0.88; }
  .btn-subject  { background: linear-gradient(135deg, #667eea, #764ba2); color: white; }
  .btn-multiple { background: linear-gradient(135deg, #f6d365, #fda085); color: white; }
</style>
</head>
<body>
<div class="card">
  <h1>🖼️ 이미지 퀴즈</h1>
  <p>총 {{.Total}}장의 이미지로 퀴즈를 시작합니다.<br>모드를 선택하세요!</p>
  <a class="btn btn-subject"  href="/start?mode=subject">✏️ 주관식</a>
  <a class="btn btn-multiple" href="/start?mode=multiple">🔢 객관식 (4지선다)</a>
</div>
</body>
</html>
`))

var quizTmpl = template.Must(template.New("quiz").Parse(`
<!DOCTYPE html>
<html lang="ko">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>이미지 퀴즈</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: 'Segoe UI', sans-serif;
    background: #f0f4f8; min-height: 100vh;
    display: flex; flex-direction: column;
    align-items: center; padding: 30px 16px;
  }
  h1 { font-size: 1.6rem; color: #2d3748; margin-bottom: 6px; }
  .progress-bar-wrap {
    width: 100%; max-width: 560px;
    background: #e2e8f0; border-radius: 999px;
    height: 10px; margin-bottom: 16px; overflow: hidden;
  }
  .progress-bar {
    height: 100%; border-radius: 999px;
    background: linear-gradient(90deg, #667eea, #764ba2);
    transition: width 0.3s;
  }
  .info {
    display: flex; gap: 12px; margin-bottom: 16px;
    font-size: 0.9rem; flex-wrap: wrap; justify-content: center;
  }
  .badge {
    background: white; border-radius: 8px;
    padding: 5px 12px; box-shadow: 0 1px 4px rgba(0,0,0,0.1);
    font-weight: 600; color: #4a5568;
  }
  .card {
    background: white; border-radius: 16px;
    box-shadow: 0 4px 20px rgba(0,0,0,0.1);
    padding: 24px; width: 100%; max-width: 560px;
  }
  .img-wrap {
    width: 100%; text-align: center; margin-bottom: 20px;
    background: #f7fafc; border-radius: 12px; padding: 12px;
    min-height: 180px; display: flex;
    align-items: center; justify-content: center;
  }
  .img-wrap img { max-width: 100%; max-height: 320px; object-fit: contain; border-radius: 8px; }
  .input-row { display: flex; gap: 10px; }
  input[type="text"] {
    flex: 1; padding: 12px 16px;
    border: 2px solid #e2e8f0; border-radius: 10px;
    font-size: 1rem; outline: none; transition: border 0.2s;
  }
  input[type="text"]:focus { border-color: #667eea; }
  .choices { display: flex; flex-direction: column; gap: 10px; }
  .choice-btn {
    width: 100%; padding: 14px 18px;
    border: 2px solid #e2e8f0; border-radius: 12px;
    background: white; font-size: 1rem; text-align: left;
    cursor: pointer; transition: all 0.15s; font-weight: 500; color: #2d3748;
  }
  .choice-btn:hover { border-color: #667eea; background: #ebf4ff; }
  button[type="submit"]:not(.choice-btn) {
    padding: 12px 22px; border: none;
    background: linear-gradient(135deg, #667eea, #764ba2);
    color: white; border-radius: 10px;
    font-size: 1rem; font-weight: 600;
    cursor: pointer; transition: opacity 0.2s;
  }
  button[type="submit"]:not(.choice-btn):hover { opacity: 0.88; }
  .result-box {
    margin-top: 16px; padding: 14px 18px;
    border-radius: 12px; font-size: 1rem; font-weight: 600; text-align: center;
  }
  .correct-box { background: #c6f6d5; color: #22543d; }
  .wrong-box   { background: #fed7d7; color: #742a2a; }
  .next-btn {
    display: block; width: 100%; margin-top: 12px;
    text-align: center; text-decoration: none; padding: 12px;
    border-radius: 10px; background: #2d3748; color: white;
    font-weight: 600; font-size: 1rem;
  }
  .next-btn:hover { background: #1a202c; }
  .mode-tag {
    font-size: 0.8rem; background: #ebf4ff; color: #667eea;
    border-radius: 6px; padding: 3px 10px; font-weight: 600;
  }
</style>
</head>
<body>
<h1>🖼️ 이미지 퀴즈 <span class="mode-tag">{{if eq .Mode "subject"}}주관식{{else}}객관식{{end}}</span></h1>

<div class="info">
  <span class="badge">📋 {{.Current}} / {{.Total}}</span>
  <span class="badge">⭐ {{.Score}}</span>
  <span class="badge">❌ {{.WrongCount}}</span>
</div>

<div class="progress-bar-wrap">
  <div class="progress-bar" style="width: {{.Progress}}%"></div>
</div>

<div class="card">
  <div class="img-wrap">
    <img src="/images/{{.ImageFile}}" alt="퀴즈 이미지">
  </div>

  {{if .ShowResult}}
    {{if .IsCorrect}}
      <div class="result-box correct-box">✅ 정답입니다!</div>
    {{else}}
      <div class="result-box wrong-box">❌ 틀렸습니다. 정답: <strong>{{.Answer}}</strong></div>
    {{end}}
    <a class="next-btn" href="/next">다음 문제 →</a>

  {{else if eq .Mode "subject"}}
    <form method="POST" action="/answer">
      <div class="input-row">
        <input type="text" name="answer" placeholder="파일명 입력 (확장자 제외)" autofocus autocomplete="off">
        <button type="submit">제출</button>
      </div>
    </form>

  {{else}}
    <form method="POST" action="/answer">
      <div class="choices">
        {{range .Choices}}
        <button class="choice-btn" type="submit" name="answer" value="{{.}}">{{.}}</button>
        {{end}}
      </div>
    </form>
  {{end}}
</div>

</body>
</html>
`))

var resultTmpl = template.Must(template.New("result").Parse(`
<!DOCTYPE html>
<html lang="ko">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>퀴즈 결과</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: 'Segoe UI', sans-serif;
    background: #f0f4f8; min-height: 100vh;
    display: flex; flex-direction: column;
    align-items: center; padding: 30px 16px;
  }
  .score-card {
    background: white; border-radius: 20px;
    box-shadow: 0 4px 20px rgba(0,0,0,0.1);
    padding: 36px; max-width: 600px; width: 100%;
    text-align: center; margin-bottom: 24px;
  }
  h1 { font-size: 1.8rem; color: #2d3748; margin-bottom: 16px; }
  .big-score { font-size: 3.5rem; font-weight: 800; color: #667eea; }
  .sub { font-size: 1rem; color: #718096; margin-top: 6px; }
  .btn-wrap { display: flex; gap: 12px; justify-content: center; margin-top: 24px; flex-wrap: wrap; }
  .btn { padding: 13px 28px; border-radius: 12px; text-decoration: none; font-weight: 700; font-size: 0.95rem; }
  .btn-home    { background: #667eea; color: white; }
  .btn-restart { background: #e2e8f0; color: #2d3748; }
  h2 { font-size: 1.2rem; color: #e53e3e; margin: 16px 0 12px; text-align: left; max-width: 600px; width: 100%; }
  .wrong-item {
    background: white; border-radius: 12px;
    padding: 12px 16px; margin-bottom: 10px;
    box-shadow: 0 2px 8px rgba(0,0,0,0.07);
    display: flex; gap: 14px; align-items: center;
    max-width: 600px; width: 100%;
  }
  .wrong-item img { width: 80px; height: 60px; object-fit: cover; border-radius: 8px; flex-shrink: 0; }
  .wrong-info { text-align: left; }
  .wrong-info .ans  { font-weight: 700; color: #2d3748; }
  .wrong-info .user { color: #e53e3e; font-size: 0.88rem; margin-top: 3px; }
</style>
</head>
<body>
<div class="score-card">
  <h1>🎉 퀴즈 완료!</h1>
  <div class="big-score">{{.Score}} / {{.Total}}</div>
  <div class="sub">정답률 {{.Rate}}%</div>
  <div class="btn-wrap">
    <a class="btn btn-home"    href="/">🏠 모드 선택으로</a>
    <a class="btn btn-restart" href="/restart">🔄 같은 모드로 재시작</a>
  </div>
</div>

{{if .WrongItems}}
<h2>❌ 틀린 문제 ({{len .WrongItems}}개)</h2>
{{range .WrongItems}}
<div class="wrong-item">
  <img src="/images/{{.ImagePath}}" alt="">
  <div class="wrong-info">
    <div class="ans">정답: {{.Answer}}</div>
    <div class="user">내 답: {{.UserInput}}</div>
  </div>
</div>
{{end}}
{{end}}

</body>
</html>
`))

// =====================
// 핸들러
// =====================

type QuizData struct {
	ImageFile  string
	Current    int
	Total      int
	Score      int
	WrongCount int
	Progress   float64
	ShowResult bool
	IsCorrect  bool
	Answer     string
	Mode       Mode
	Choices    []string
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	total := 0
	if session != nil {
		total = len(session.Images)
	}
	mu.Unlock()
	selectTmpl.Execute(w, map[string]int{"Total": total})
}

func startHandler(w http.ResponseWriter, r *http.Request) {
	mode := Mode(r.URL.Query().Get("mode"))
	if mode != ModeSubject && mode != ModeMultiple {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	images, _ := loadImages("images")
	mu.Lock()
	session = &QuizSession{Images: images, Mode: mode}
	mu.Unlock()
	http.Redirect(w, r, "/quiz", http.StatusFound)
}

func quizHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	if session == nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	if session.Done {
		http.Redirect(w, r, "/result", http.StatusFound)
		return
	}

	current := session.Images[session.Current]
	total := len(session.Images)
	progress := float64(session.Current) / float64(total) * 100

	data := QuizData{
		ImageFile:  current,
		Current:    session.Current + 1,
		Total:      total,
		Score:      session.Score,
		WrongCount: len(session.Wrong),
		Progress:   progress,
		Mode:       session.Mode,
	}
	if session.Mode == ModeMultiple {
		data.Choices = makeChoices(session.Images, session.Current)
	}
	quizTmpl.Execute(w, data)
}

func answerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/quiz", http.StatusFound)
		return
	}
	r.ParseForm()
	userInput := r.FormValue("answer")

	mu.Lock()
	defer mu.Unlock()

	if session == nil || session.Done {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	current := session.Images[session.Current]
	isCorrect := checkAnswer(current, userInput)
	answer := getAnswer(current)

	if isCorrect {
		session.Score++
	} else {
		session.Wrong = append(session.Wrong, WrongItem{
			ImagePath: current,
			Answer:    answer,
			UserInput: userInput,
		})
	}

	total := len(session.Images)
	progress := float64(session.Current) / float64(total) * 100

	data := QuizData{
		ImageFile:  current,
		Current:    session.Current + 1,
		Total:      total,
		Score:      session.Score,
		WrongCount: len(session.Wrong),
		Progress:   progress,
		ShowResult: true,
		IsCorrect:  isCorrect,
		Answer:     answer,
		Mode:       session.Mode,
	}
	quizTmpl.Execute(w, data)
}

func nextHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	if session == nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	session.Current++
	if session.Current >= len(session.Images) {
		session.Done = true
		http.Redirect(w, r, "/result", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/quiz", http.StatusFound)
}

func resultHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	if session == nil {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	total := len(session.Images)
	rate := 0
	if total > 0 {
		rate = int(float64(session.Score) / float64(total) * 100)
	}
	data := struct {
		Score      int
		Total      int
		Rate       int
		WrongItems []WrongItem
	}{session.Score, total, rate, session.Wrong}
	resultTmpl.Execute(w, data)
}

func restartHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	mode := ModeSubject
	if session != nil {
		mode = session.Mode
	}
	mu.Unlock()
	images, _ := loadImages("images")
	mu.Lock()
	session = &QuizSession{Images: images, Mode: mode}
	mu.Unlock()
	http.Redirect(w, r, "/quiz", http.StatusFound)
}

// =====================
// main
// =====================

func main() {
	images, err := loadImages("images")
	if err != nil || len(images) == 0 {
		fmt.Println("❌ images 폴더를 확인해주세요.")
		os.Exit(1)
	}
	session = &QuizSession{Images: images, Mode: ModeSubject}
	fmt.Printf("✅ 이미지 %d장 로드 완료!\n", len(images))

	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/start", startHandler)
	http.HandleFunc("/quiz", quizHandler)
	http.HandleFunc("/answer", answerHandler)
	http.HandleFunc("/next", nextHandler)
	http.HandleFunc("/result", resultHandler)
	http.HandleFunc("/restart", restartHandler)
	http.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir("images"))))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Println("🚀 서버 시작: http://localhost:" + port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Println("서버 오류:", err)
	}
}
