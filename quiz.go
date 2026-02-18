package main

import (
	"encoding/json"
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

type QuizSession struct {
	Images  []string
	Current int
	Score   int
	Wrong   []WrongItem
	Done    bool
}

type WrongItem struct {
	ImagePath string
	Answer    string
	UserInput string
}

// 전역 세션 (단일 사용자용)
var (
	session *QuizSession
	mu      sync.Mutex
)

// =====================
// 이미지 폴더 로드
// =====================

func loadImages(dir string) ([]string, error) {

	var cnt int
	validExt := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
	}

	var images []string
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if cnt == 30 {
			break
		}

		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if validExt[ext] {
			images = append(images, e.Name())
			cnt++
		}
	}

	// 랜덤 셔플
	rand.New(rand.NewSource(time.Now().UnixNano()))
	rand.Shuffle(len(images), func(i, j int) {
		images[i], images[j] = images[j], images[i]
	})

	return images, nil
}

// =====================
// 정답 확인
// =====================

func getAnswer(filename string) string {
	ext := filepath.Ext(filename)
	return strings.TrimSuffix(filename, ext)
}

func checkAnswer(filename, userInput string) bool {
	answer := getAnswer(filename)
	return strings.EqualFold(strings.TrimSpace(userInput), strings.TrimSpace(answer))
}

// =====================
// HTML 템플릿
// =====================

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
    background: #f0f4f8;
    min-height: 100vh;
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 30px 16px;
  }
  h1 { font-size: 1.8rem; color: #2d3748; margin-bottom: 6px; }
  .progress-bar-wrap {
    width: 100%; max-width: 560px;
    background: #e2e8f0; border-radius: 999px;
    height: 10px; margin-bottom: 20px; overflow: hidden;
  }
  .progress-bar {
    height: 100%; border-radius: 999px;
    background: linear-gradient(90deg, #667eea, #764ba2);
    transition: width 0.3s;
  }
  .info {
    display: flex; gap: 20px; margin-bottom: 20px;
    font-size: 0.95rem; color: #4a5568;
  }
  .badge {
    background: white; border-radius: 8px;
    padding: 6px 14px; box-shadow: 0 1px 4px rgba(0,0,0,0.1);
    font-weight: 600;
  }
  .card {
    background: white; border-radius: 16px;
    box-shadow: 0 4px 20px rgba(0,0,0,0.1);
    padding: 28px; width: 100%; max-width: 560px;
  }
  .img-wrap {
    width: 100%; text-align: center;
    margin-bottom: 24px;
    background: #f7fafc; border-radius: 12px;
    padding: 12px; min-height: 200px;
    display: flex; align-items: center; justify-content: center;
  }
  .img-wrap img {
    max-width: 100%; max-height: 340px;
    object-fit: contain; border-radius: 8px;
  }
  .input-row {
    display: flex; gap: 10px;
  }
  input[type="text"] {
    flex: 1; padding: 12px 16px;
    border: 2px solid #e2e8f0; border-radius: 10px;
    font-size: 1rem; outline: none; transition: border 0.2s;
  }
  input[type="text"]:focus { border-color: #667eea; }
  button {
    padding: 12px 22px; border: none;
    background: linear-gradient(135deg, #667eea, #764ba2);
    color: white; border-radius: 10px;
    font-size: 1rem; font-weight: 600;
    cursor: pointer; transition: opacity 0.2s;
  }
  button:hover { opacity: 0.88; }
  .result-box {
    margin-top: 20px; padding: 16px 20px;
    border-radius: 12px; font-size: 1.05rem; font-weight: 600;
    text-align: center;
  }
  .correct { background: #c6f6d5; color: #22543d; }
  .wrong   { background: #fed7d7; color: #742a2a; }
  .next-btn {
    display: block; width: 100%; margin-top: 14px;
    text-align: center; text-decoration: none;
    padding: 12px; border-radius: 10px;
    background: #2d3748; color: white;
    font-weight: 600; font-size: 1rem;
  }
  .next-btn:hover { background: #1a202c; }
</style>
</head>
<body>
<h1>🖼️ 이미지 퀴즈</h1>

<div class="info">
  <span class="badge">📋 {{.Current}} / {{.Total}}</span>
  <span class="badge">⭐ 점수: {{.Score}}</span>
  <span class="badge">❌ 오답: {{.WrongCount}}</span>
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
      <div class="result-box correct">✅ 정답입니다!</div>
    {{else}}
      <div class="result-box wrong">❌ 틀렸습니다. 정답: <strong>{{.Answer}}</strong></div>
    {{end}}
    <a class="next-btn" href="/next">다음 문제 →</a>
  {{else}}
    <form method="POST" action="/answer">
      <div class="input-row">
        <input type="text" name="answer" placeholder="파일명을 입력하세요 (확장자 제외)" autofocus autocomplete="off">
        <button type="submit">제출</button>
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
<title>퀴즈 결과</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body {
    font-family: 'Segoe UI', sans-serif;
    background: #f0f4f8; min-height: 100vh;
    display: flex; flex-direction: column;
    align-items: center; padding: 30px 16px;
  }
  h1 { font-size: 2rem; color: #2d3748; margin-bottom: 10px; }
  .score-card {
    background: white; border-radius: 20px;
    box-shadow: 0 4px 20px rgba(0,0,0,0.1);
    padding: 36px; max-width: 600px; width: 100%;
    text-align: center; margin-bottom: 24px;
  }
  .big-score { font-size: 3.5rem; font-weight: 800; color: #667eea; }
  .sub { font-size: 1rem; color: #718096; margin-top: 6px; }
  h2 { font-size: 1.3rem; color: #e53e3e; margin: 24px 0 14px; text-align: left; }
  .wrong-item {
    background: white; border-radius: 12px;
    padding: 14px 18px; margin-bottom: 12px;
    box-shadow: 0 2px 8px rgba(0,0,0,0.07);
    display: flex; gap: 16px; align-items: center;
  }
  .wrong-item img { width: 80px; height: 60px; object-fit: cover; border-radius: 8px; }
  .wrong-info { text-align: left; }
  .wrong-info .ans { font-weight: 700; color: #2d3748; }
  .wrong-info .user { color: #e53e3e; font-size: 0.9rem; }
  .restart {
    display: inline-block; padding: 14px 40px;
    background: linear-gradient(135deg, #667eea, #764ba2);
    color: white; border-radius: 12px;
    text-decoration: none; font-weight: 700; font-size: 1.05rem;
    margin-top: 10px;
  }
  .wrap { max-width: 600px; width: 100%; }
</style>
</head>
<body>
<div class="wrap">
  <div class="score-card">
    <h1>🎉 퀴즈 완료!</h1>
    <div class="big-score">{{.Score}} / {{.Total}}</div>
    <div class="sub">정답률 {{.Rate}}%</div>
    <br>
    <a class="restart" href="/restart">🔄 다시 시작</a>
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
</div>
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
}

// 현재 퀴즈 화면
func quizHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()

	if session == nil || len(session.Images) == 0 {
		http.Error(w, "images 폴더가 없거나 이미지가 없습니다.", http.StatusInternalServerError)
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
	}
	quizTmpl.Execute(w, data)
}

// 정답 제출
func answerHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Redirect(w, r, "/", http.StatusFound)
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
	}
	quizTmpl.Execute(w, data)
}

// 다음 문제
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
	http.Redirect(w, r, "/", http.StatusFound)
}

// 결과 화면
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
	}{
		Score:      session.Score,
		Total:      total,
		Rate:       rate,
		WrongItems: session.Wrong,
	}
	resultTmpl.Execute(w, data)
}

// 다시 시작
func restartHandler(w http.ResponseWriter, r *http.Request) {
	images, err := loadImages("images")
	if err != nil || len(images) == 0 {
		http.Error(w, "images 폴더를 읽을 수 없습니다.", http.StatusInternalServerError)
		return
	}
	mu.Lock()
	session = &QuizSession{Images: images}
	mu.Unlock()
	http.Redirect(w, r, "/", http.StatusFound)
}

// 세션 상태 API (선택)
func statusHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if session == nil {
		json.NewEncoder(w).Encode(map[string]any{"error": "no session"})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"current": session.Current,
		"total":   len(session.Images),
		"score":   session.Score,
		"wrong":   len(session.Wrong),
	})
}

// =====================
// main
// =====================

func main() {
	// images 폴더 로드
	images, err := loadImages("images")
	if err != nil {
		fmt.Println("❌ images 폴더를 찾을 수 없습니다. images/ 폴더를 quiz.go 옆에 만들어주세요.")
		os.Exit(1)
	}
	if len(images) == 0 {
		fmt.Println("❌ images 폴더에 이미지가 없습니다.")
		os.Exit(1)
	}

	session = &QuizSession{Images: images}
	fmt.Printf("✅ 이미지 %d장 로드 완료!\n", len(images))

	// 라우팅
	http.HandleFunc("/", quizHandler)
	http.HandleFunc("/answer", answerHandler)
	http.HandleFunc("/next", nextHandler)
	http.HandleFunc("/result", resultHandler)
	http.HandleFunc("/restart", restartHandler)
	http.HandleFunc("/status", statusHandler)

	// images 폴더 정적 서빙
	http.Handle("/images/", http.StripPrefix("/images/", http.FileServer(http.Dir("images"))))

	fmt.Println("🚀 서버 시작: http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		fmt.Println("서버 오류:", err)
	}
}
