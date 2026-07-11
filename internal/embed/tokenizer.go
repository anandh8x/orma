package embed

import (
	"bufio"
	"os"
	"strings"
	"unicode"
)

// BertWordPiece is a minimal WordPiece tokenizer for MiniLM/BERT vocab.txt.
type BertWordPiece struct {
	vocab    map[string]int32
	unkID    int32
	clsID    int32
	sepID    int32
	padID    int32
	maxLen   int
}

// LoadVocab loads a BERT-style vocab.txt.
func LoadVocab(path string, maxLen int) (*BertWordPiece, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if maxLen <= 0 {
		maxLen = 128
	}
	t := &BertWordPiece{
		vocab:  map[string]int32{},
		maxLen: maxLen,
		unkID:  100, // typical BERT [UNK]
		clsID:  101,
		sepID:  102,
		padID:  0,
	}
	sc := bufio.NewScanner(f)
	var id int32
	for sc.Scan() {
		tok := sc.Text()
		t.vocab[tok] = id
		switch tok {
		case "[UNK]":
			t.unkID = id
		case "[CLS]":
			t.clsID = id
		case "[SEP]":
			t.sepID = id
		case "[PAD]":
			t.padID = id
		}
		id++
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return t, nil
}

// Encode returns input_ids, attention_mask, token_type_ids for one sentence.
func (t *BertWordPiece) Encode(text string) (ids, mask, types []int64) {
	pieces := t.tokenize(text)
	// [CLS] + pieces + [SEP], truncate
	maxPieces := t.maxLen - 2
	if len(pieces) > maxPieces {
		pieces = pieces[:maxPieces]
	}
	ids = make([]int64, 0, len(pieces)+2)
	ids = append(ids, int64(t.clsID))
	for _, p := range pieces {
		if id, ok := t.vocab[p]; ok {
			ids = append(ids, int64(id))
		} else {
			ids = append(ids, int64(t.unkID))
		}
	}
	ids = append(ids, int64(t.sepID))
	mask = make([]int64, len(ids))
	types = make([]int64, len(ids))
	for i := range ids {
		mask[i] = 1
		types[i] = 0
	}
	return ids, mask, types
}

func (t *BertWordPiece) tokenize(text string) []string {
	text = basicClean(text)
	var out []string
	for _, tok := range strings.Fields(text) {
		out = append(out, t.wordPiece(tok)...)
	}
	return out
}

func basicClean(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) {
			continue
		}
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			b.WriteByte(' ')
			if unicode.IsPunct(r) {
				// keep punct as separate-ish via spaces
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (t *BertWordPiece) wordPiece(token string) []string {
	if token == "" {
		return nil
	}
	if _, ok := t.vocab[token]; ok {
		return []string{token}
	}
	var chars []string
	for _, r := range token {
		chars = append(chars, string(r))
	}
	start := 0
	var sub []string
	for start < len(chars) {
		end := len(chars)
		var cur string
		found := false
		for end > start {
			substr := strings.Join(chars[start:end], "")
			if start > 0 {
				substr = "##" + substr
			}
			if _, ok := t.vocab[substr]; ok {
				cur = substr
				found = true
				break
			}
			end--
		}
		if !found {
			return []string{"[UNK]"}
		}
		sub = append(sub, cur)
		start = end
	}
	return sub
}
