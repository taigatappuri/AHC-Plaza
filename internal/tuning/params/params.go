// Package params は限定されたC++宣言の注釈と初期値だけを扱います。
package params

import (
	"crypto/sha256"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const Version = 1
const MaxSource = 8 << 20

type Parameter struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Current float64  `json:"current"`
	Low     *float64 `json:"low"`
	High    *float64 `json:"high"`
	Step    *float64 `json:"step"`
	Log     bool     `json:"log"`
	Enabled bool     `json:"enabled"`
	Line    int      `json:"line"`
	Start   int      `json:"start"`
	End     int      `json:"end"`
	Token   string   `json:"token"`
}
type Scan struct {
	Hash       string      `json:"hash"`
	Source     string      `json:"source"`
	Parameters []Parameter `json:"parameters"`
}

var declaration = regexp.MustCompile(`^\s*(?:(?:static|const|constexpr)\s+)*(int|long\s+long|float|double)\s+([A-Za-z_][A-Za-z_0-9]*)\s*=\s*([+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?(?:[fF]|[lL]{2})?)\s*;\s*$`)

func Hash(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

// Parse は文字列・コメント・条件付きプリプロセッサを走査し、行単位の宣言のみ解析します。
func Parse(source []byte) (Scan, error) {
	out := Scan{Hash: Hash(source), Source: string(source), Parameters: []Parameter{}}
	if len(source) > MaxSource {
		return out, fmt.Errorf("ソースは8 MiB以下にしてください")
	}
	s := string(source)
	line, start, depth, conditional := 1, 0, 0, 0
	names := map[string]bool{}
	for i := 0; i < len(s); {
		c := s[i]
		if c == '\n' {
			line++
			start = i + 1
			i++
			continue
		}
		if c == '#' && strings.TrimSpace(s[start:i]) == "" {
			end := i
			for end < len(s) {
				n := strings.IndexByte(s[end:], '\n')
				if n < 0 {
					end = len(s)
					break
				}
				end += n
				previous := end - 1
				if previous >= 0 && s[previous] == '\r' {
					previous--
				}
				if previous >= 0 && s[previous] == '\\' {
					end++
					continue
				}
				break
			}
			text := s[i:end]
			if strings.Contains(text, "@tune") {
				return out, fmt.Errorf("%d行: プリプロセッサ内の@tuneには対応しません", line)
			}
			words := strings.Fields(strings.TrimPrefix(text, "#"))
			if len(words) > 0 {
				switch words[0] {
				case "if", "ifdef", "ifndef":
					conditional++
				case "endif":
					conditional--
					if conditional < 0 {
						return out, fmt.Errorf("%d行: #endifが不正です", line)
					}
				}
			}
			line += strings.Count(text, "\n")
			if n := strings.LastIndexByte(text, '\n'); n >= 0 {
				start = i + n + 1
			}
			i = end
			continue
		}
		if strings.HasPrefix(s[i:], "//") {
			end := strings.IndexByte(s[i:], '\n')
			if end < 0 {
				end = len(s)
			} else {
				end += i
			}
			continued := false
			for end < len(s) {
				previous := end - 1
				if previous >= 0 && s[previous] == '\r' {
					previous--
				}
				if previous < 0 || s[previous] != '\\' {
					break
				}
				continued = true
				next := strings.IndexByte(s[end+1:], '\n')
				if next < 0 {
					end = len(s)
					break
				}
				end += next + 1
			}
			comment := s[i+2 : end]
			if continued && strings.Contains(comment, "@tune") {
				return out, fmt.Errorf("%d行: 行継続コメント内の注釈は未対応です", line)
			}
			if strings.Contains(comment, "@tune") {
				if depth != 0 || conditional != 0 || strings.Contains(s[start:i], "\\") {
					return out, fmt.Errorf("%d行: グローバルの条件分岐外で1行の宣言を使ってください", line)
				}
				note := strings.TrimSpace(comment)
				if !strings.HasPrefix(note, "@tune") || (len(note) > 5 && note[5] != ' ' && note[5] != '\t' && note[5] != '\r') {
					return out, fmt.Errorf("%d行: // @tune LOW HIGH の形式にしてください", line)
				}
				match := declaration.FindStringSubmatchIndex(s[start:i])
				if match == nil {
					return out, fmt.Errorf("%d行: 例 constexpr int WIDTH = 256; // @tune 64 512", line)
				}
				get := func(n int) string { return s[start+match[2*n] : start+match[2*n+1]] }
				p := Parameter{Name: get(2), Type: strings.Join(strings.Fields(get(1)), " "), Line: line, Start: start + match[6], End: start + match[7], Token: get(3), Enabled: true}
				if names[p.Name] {
					return out, fmt.Errorf("%d行: パラメータ名%sが重複しています", line, p.Name)
				}
				names[p.Name] = true
				raw := strings.TrimRight(p.Token, "fFlL")
				value, err := strconv.ParseFloat(raw, 64)
				if err != nil {
					return out, err
				}
				p.Current = value
				unsigned := strings.TrimLeft(raw, "+-")
				if p.integer() && len(unsigned) > 1 && unsigned[0] == '0' {
					return out, fmt.Errorf("%d行: 先頭に0のある整数は未対応です", line)
				}
				if p.Type == "float" {
					p.Current = float64(float32(value))
				}
				if err := p.checkValue(value); err != nil {
					return out, fmt.Errorf("%d行: %w", line, err)
				}
				if p.integer() && (strings.ContainsAny(raw, ".eE") || strings.ContainsAny(p.Token, "fF")) {
					return out, fmt.Errorf("%d行: 整数リテラルを指定してください", line)
				}
				if !p.integer() && strings.ContainsAny(p.Token, "lL") {
					return out, fmt.Errorf("%d行: 実数のLLサフィックスは未対応です", line)
				}
				words := strings.Fields(note[5:])
				if len(words) > 0 {
					if len(words) < 2 {
						return out, fmt.Errorf("%d行: 下限と上限が必要です", line)
					}
					p.Low, err = number(words[0])
					if err != nil {
						return out, err
					}
					p.High, err = number(words[1])
					if err != nil {
						return out, err
					}
					for _, w := range words[2:] {
						if w == "log" && !p.Log {
							p.Log = true
						} else if strings.HasPrefix(w, "step=") && p.Step == nil {
							p.Step, err = number(w[5:])
							if err != nil {
								return out, err
							}
						} else {
							return out, fmt.Errorf("%d行: 不明な指定 %s", line, w)
						}
					}
					if err := p.Validate(); err != nil {
						return out, fmt.Errorf("%d行: %w", line, err)
					}
				}
				out.Parameters = append(out.Parameters, p)
			}
			line += strings.Count(s[i:end], "\n")
			if n := strings.LastIndexByte(s[i:end], '\n'); n >= 0 {
				start = i + n + 1
			}
			i = end
			continue
		}
		if strings.HasPrefix(s[i:], "/*") {
			n := strings.Index(s[i+2:], "*/")
			if n < 0 {
				return out, fmt.Errorf("%d行: コメントが閉じていません", line)
			}
			end := i + 2 + n + 2
			if strings.Contains(s[i:end], "@tune") {
				return out, fmt.Errorf("%d行: 注釈には // @tune を使ってください", line)
			}
			text := s[i:end]
			line += strings.Count(text, "\n")
			if n := strings.LastIndexByte(text, '\n'); n >= 0 {
				start = i + n + 1
			}
			i = end
			continue
		}
		if strings.HasPrefix(s[i:], `R"`) {
			n := strings.IndexByte(s[i+2:], '(')
			if n < 0 || n > 16 {
				return out, fmt.Errorf("%d行: raw文字列が不正です", line)
			}
			delimiter := s[i+2 : i+2+n]
			closing := ")" + delimiter + `"`
			from := i + 3 + n
			end := strings.Index(s[from:], closing)
			if end < 0 {
				return out, fmt.Errorf("%d行: raw文字列が閉じていません", line)
			}
			end += from + len(closing)
			text := s[i:end]
			line += strings.Count(text, "\n")
			if n := strings.LastIndexByte(text, '\n'); n >= 0 {
				start = i + n + 1
			}
			i = end
			continue
		}
		if c == '\'' && i > 0 && i+1 < len(s) && strings.ContainsRune("0123456789abcdefABCDEF", rune(s[i-1])) && strings.ContainsRune("0123456789abcdefABCDEF", rune(s[i+1])) {
			i++
			continue
		}
		if c == '"' || c == '\'' {
			quote := c
			i++
			closed := false
			for i < len(s) {
				if s[i] == '\\' {
					if i+1 < len(s) && s[i+1] == '\n' {
						line++
						start = i + 2
					}
					i += 2
					continue
				}
				if s[i] == quote {
					i++
					closed = true
					break
				}
				if s[i] == '\n' {
					return out, fmt.Errorf("%d行: 文字列が不正です", line)
				}
				i++
			}
			if !closed {
				return out, fmt.Errorf("%d行: 文字列が閉じていません", line)
			}
			continue
		}
		if c == '{' {
			depth++
		}
		if c == '}' {
			depth--
			if depth < 0 {
				return out, fmt.Errorf("%d行: スコープが不正です", line)
			}
		}
		i++
	}
	return out, nil
}
func number(s string) (*float64, error) {
	v, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, fmt.Errorf("有限数を指定してください: %s", s)
	}
	return &v, nil
}
func (p Parameter) integer() bool { return p.Type == "int" || p.Type == "long long" }
func (p Parameter) checkValue(v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("%s: 有限数が必要です", p.Name)
	}
	if p.integer() {
		if math.Trunc(v) != v || math.Abs(v) > 9007199254740991 {
			return fmt.Errorf("%s: 整数は±(2^53-1)以内です", p.Name)
		}
		if p.Type == "int" && (v < math.MinInt32 || v > math.MaxInt32) {
			return fmt.Errorf("%s: int範囲外です", p.Name)
		}
	}
	if p.Type == "float" && math.Abs(v) > math.MaxFloat32 {
		return fmt.Errorf("%s: float範囲外です", p.Name)
	}
	return nil
}
func (p Parameter) Validate() error {
	if !p.Enabled {
		return nil
	}
	if p.Low == nil || p.High == nil {
		return fmt.Errorf("%s: 探索範囲が未入力です", p.Name)
	}
	for _, v := range []float64{*p.Low, *p.High} {
		if e := p.checkValue(v); e != nil {
			return e
		}
	}
	if *p.Low > *p.High {
		return fmt.Errorf("%s: 下限が上限を超えています", p.Name)
	}
	if p.Log && (*p.Low <= 0 || p.Step != nil) {
		return fmt.Errorf("%s: logは正の範囲でstepと併用できません", p.Name)
	}
	if p.Step != nil {
		v := *p.Step
		if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) || (p.integer() && math.Trunc(v) != v) {
			return fmt.Errorf("%s: 刻み幅が不正です", p.Name)
		}
		rat := func(v float64) *big.Rat {
			r, _ := new(big.Rat).SetString(strconv.FormatFloat(v, 'g', -1, 64))
			return r
		}
		q := new(big.Rat).Quo(new(big.Rat).Sub(rat(*p.High), rat(*p.Low)), rat(v))
		if !q.IsInt() {
			return fmt.Errorf("%s: 上限と下限の差を刻み幅の整数倍にしてください", p.Name)
		}
	}
	return nil
}

// Resolve はサーバー側で検出した位置・型を保持し、探索設定だけを適用します。
func Resolve(scan Scan, overrides []Parameter) ([]Parameter, error) {
	byName := map[string]Parameter{}
	for _, p := range overrides {
		if _, ok := byName[p.Name]; ok {
			return nil, fmt.Errorf("パラメータが重複しています")
		}
		byName[p.Name] = p
	}
	result := append([]Parameter(nil), scan.Parameters...)
	enabled := 0
	for i := range result {
		p := &result[i]
		if o, ok := byName[p.Name]; ok {
			p.Low = o.Low
			p.High = o.High
			p.Step = o.Step
			p.Log = o.Log
			p.Enabled = o.Enabled
			delete(byName, p.Name)
		}
		if err := p.Validate(); err != nil {
			return nil, err
		}
		if p.Enabled {
			enabled++
		}
	}
	if len(byName) > 0 {
		return nil, fmt.Errorf("不明なパラメータです")
	}
	if enabled == 0 {
		return nil, fmt.Errorf("探索対象を1つ以上選んでください")
	}
	return result, nil
}
func Generate(source []byte, hash string, parameters []Parameter, values map[string]float64) ([]byte, map[string]float64, error) {
	if Hash(source) != hash {
		return nil, nil, fmt.Errorf("原本ハッシュが一致しません")
	}
	out := append([]byte(nil), source...)
	actual := map[string]float64{}
	ps := append([]Parameter(nil), parameters...)
	sort.Slice(ps, func(i, j int) bool { return ps[i].Start > ps[j].Start })
	for _, p := range ps {
		v, ok := values[p.Name]
		if !ok {
			continue
		}
		if p.Start < 0 || p.End > len(source) || p.Start >= p.End || string(source[p.Start:p.End]) != p.Token {
			return nil, nil, fmt.Errorf("%s: 原本トークンが一致しません", p.Name)
		}
		if e := p.checkValue(v); e != nil {
			return nil, nil, e
		}
		token := ""
		switch p.Type {
		case "int":
			token = strconv.FormatInt(int64(v), 10)
		case "long long":
			token = strconv.FormatInt(int64(v), 10) + "LL"
		case "float":
			v = float64(float32(v))
			token = strconv.FormatFloat(v, 'g', -1, 32)
			if !strings.ContainsAny(token, ".eE") {
				token += ".0"
			}
			token += "f"
		case "double":
			token = strconv.FormatFloat(v, 'g', -1, 64)
			if !strings.ContainsAny(token, ".eE") {
				token += ".0"
			}
		default:
			return nil, nil, fmt.Errorf("未対応の型です")
		}
		out = append(append(append([]byte{}, out[:p.Start]...), []byte(token)...), out[p.End:]...)
		actual[p.Name] = v
	}
	if len(actual) != len(values) {
		return nil, nil, fmt.Errorf("不明なパラメータです")
	}
	return out, actual, nil
}

// ValidateValues はworkerから返った候補が固定探索空間内か確認します。
func ValidateValues(parameters []Parameter, values map[string]float64) error {
	expected := 0
	for _, p := range parameters {
		if !p.Enabled {
			continue
		}
		expected++
		v, ok := values[p.Name]
		if !ok {
			return fmt.Errorf("候補の%sがありません", p.Name)
		}
		if e := p.checkValue(v); e != nil {
			return e
		}
		if p.Low == nil || p.High == nil || v < *p.Low || v > *p.High {
			return fmt.Errorf("%sの候補が探索範囲外です", p.Name)
		}
		if p.Step != nil {
			if p.integer() {
				rat := func(x float64) *big.Rat {
					r, _ := new(big.Rat).SetString(strconv.FormatFloat(x, 'g', -1, 64))
					return r
				}
				q := new(big.Rat).Quo(new(big.Rat).Sub(rat(v), rat(*p.Low)), rat(*p.Step))
				if !q.IsInt() {
					return fmt.Errorf("%sの候補が刻みに一致しません", p.Name)
				}
				continue
			}
			q := (v - *p.Low) / *p.Step
			if math.Abs(q-math.Round(q)) > 1e-8*math.Max(1, math.Abs(q)) {
				return fmt.Errorf("%sの候補が刻みに一致しません", p.Name)
			}
		}
	}
	if len(values) != expected {
		return fmt.Errorf("候補のパラメータ数が一致しません")
	}
	return nil
}
