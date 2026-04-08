package services

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/TW-Fusion/fusion-search/app/config"
	"github.com/TW-Fusion/fusion-search/app/logging"
	ort "github.com/yalue/onnxruntime_go"

	"github.com/sugarme/tokenizer"
	"go.uber.org/zap"
)

var (
	ortInitOnce sync.Once
	ortInitErr  error
)

// RerankerService reranks search results using local ONNX inference.
type RerankerService struct {
	enabled    bool
	topK       int
	maxLength  int
	tokenizer  *tokenizer.Tokenizer
	session    *ort.DynamicAdvancedSession
	inputNames []string
	outputName string
	logger     *zap.SugaredLogger
}

func NewRerankerService(cfg *config.AppConfig) *RerankerService {
	logger := logging.GetLogger()
	topK := cfg.Rerank.TopK
	if topK <= 0 {
		topK = 5
	}

	maxLength := cfg.Rerank.MaxLength
	if maxLength <= 0 {
		maxLength = 512
	}

	r := &RerankerService{
		enabled:   cfg.Rerank.Enabled,
		topK:      topK,
		maxLength: maxLength,
		logger:    logger,
	}

	if !r.enabled {
		return r
	}

	if cfg.Rerank.OnnxModelPath == "" || cfg.Rerank.TokenizerPath == "" {
		r.logger.Warn("rerank_disabled_missing_paths")
		r.enabled = false
		return r
	}

	if cfg.Rerank.ORTLibraryPath != "" {
		ort.SetSharedLibraryPath(cfg.Rerank.ORTLibraryPath)
	}

	ortInitOnce.Do(func() {
		ortInitErr = ort.InitializeEnvironment()
	})
	if ortInitErr != nil {
		r.logger.Errorw("rerank_ort_init_failed", "error", ortInitErr)
		r.enabled = false
		return r
	}

	tok := tokenizer.NewTokenizerFromFile(cfg.Rerank.TokenizerPath)
	if tok == nil {
		r.logger.Errorw("rerank_tokenizer_load_failed", "path", cfg.Rerank.TokenizerPath)
		r.enabled = false
		return r
	}
	r.tokenizer = tok

	inputs, outputs, err := ort.GetInputOutputInfo(cfg.Rerank.OnnxModelPath)
	if err != nil {
		r.logger.Errorw("rerank_model_introspection_failed", "path", cfg.Rerank.OnnxModelPath, "error", err)
		r.enabled = false
		return r
	}
	if len(inputs) == 0 || len(outputs) == 0 {
		r.logger.Warn("rerank_model_io_empty")
		r.enabled = false
		return r
	}

	r.inputNames = make([]string, 0, len(inputs))
	for _, in := range inputs {
		r.inputNames = append(r.inputNames, in.Name)
	}
	r.outputName = outputs[0].Name

	session, err := ort.NewDynamicAdvancedSession(cfg.Rerank.OnnxModelPath, r.inputNames, []string{r.outputName}, nil)
	if err != nil {
		r.logger.Errorw("rerank_session_create_failed", "error", err)
		r.enabled = false
		return r
	}
	r.session = session
	r.logger.Infow("rerank_ready", "inputs", r.inputNames, "output", r.outputName)
	return r
}

// Rerank reranks search results with ONNX cross-encoder inference.
func (r *RerankerService) Rerank(query string, results []map[string]interface{}, topK int) []map[string]interface{} {
	if !r.enabled || r.session == nil || r.tokenizer == nil || len(results) == 0 {
		return results
	}

	if topK <= 0 {
		topK = r.topK
	}
	if topK > len(results) {
		topK = len(results)
	}

	type scoredItem struct {
		score  float64
		result map[string]interface{}
	}

	scored := make([]scoredItem, 0, len(results))
	for _, item := range results {
		title, _ := item["title"].(string)
		content, _ := item["content"].(string)
		score, err := r.inferScore(query, strings.TrimSpace(title+" "+content))
		if err != nil {
			r.logger.Warnw("rerank_inference_failed", "error", err)
			return results
		}
		item["score"] = round(score, 4)
		scored = append(scored, scoredItem{
			score:  score,
			result: item,
		})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	out := make([]map[string]interface{}, 0, topK)
	for _, item := range scored[:topK] {
		out = append(out, item.result)
	}
	return out
}

func (r *RerankerService) inferScore(query, passage string) (float64, error) {
	input := tokenizer.NewDualEncodeInput(
		tokenizer.NewInputSequence(query),
		tokenizer.NewInputSequence(passage),
	)
	enc, err := r.tokenizer.Encode(input, true)
	if err != nil {
		return 0, err
	}

	inputIDs, attentionMask, tokenTypeIDs := r.toFixedLength(enc)
	shape := ort.NewShape(1, int64(r.maxLength))

	idsTensor, err := ort.NewTensor(shape, inputIDs)
	if err != nil {
		return 0, err
	}
	defer idsTensor.Destroy()

	maskTensor, err := ort.NewTensor(shape, attentionMask)
	if err != nil {
		return 0, err
	}
	defer maskTensor.Destroy()

	typeTensor, err := ort.NewTensor(shape, tokenTypeIDs)
	if err != nil {
		return 0, err
	}
	defer typeTensor.Destroy()

	outputShape := ort.NewShape(1, 1)
	outputData := make([]float32, 1)
	outputTensor, err := ort.NewTensor(outputShape, outputData)
	if err != nil {
		return 0, err
	}
	defer outputTensor.Destroy()

	inputValues := make([]ort.Value, 0, len(r.inputNames))
	for _, name := range r.inputNames {
		switch name {
		case "input_ids":
			inputValues = append(inputValues, idsTensor)
		case "attention_mask":
			inputValues = append(inputValues, maskTensor)
		case "token_type_ids":
			inputValues = append(inputValues, typeTensor)
		default:
			return 0, fmt.Errorf("unsupported input name: %s", name)
		}
	}

	if err := r.session.Run(inputValues, []ort.Value{outputTensor}); err != nil {
		return 0, err
	}

	vals := outputTensor.GetData()
	if len(vals) == 0 {
		return 0, fmt.Errorf("empty rerank output")
	}
	return float64(vals[0]), nil
}

func (r *RerankerService) toFixedLength(enc *tokenizer.Encoding) ([]int64, []int64, []int64) {
	ids := make([]int64, r.maxLength)
	attention := make([]int64, r.maxLength)
	typeIDs := make([]int64, r.maxLength)

	rawIDs := enc.GetIds()
	rawAttention := enc.GetAttentionMask()
	rawTypeIDs := enc.GetTypeIds()

	for i := 0; i < r.maxLength; i++ {
		if i < len(rawIDs) {
			ids[i] = int64(rawIDs[i])
		}
		if i < len(rawAttention) {
			attention[i] = int64(rawAttention[i])
		}
		if i < len(rawTypeIDs) {
			typeIDs[i] = int64(rawTypeIDs[i])
		}
	}

	return ids, attention, typeIDs
}

func round(v float64, precision int) float64 {
	pow := math.Pow(10, float64(precision))
	return math.Round(v*pow) / pow
}
