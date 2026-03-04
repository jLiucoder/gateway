package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"
	"github.com/redis/go-redis/v9"
)

const (
	defaultVecSetKey  = "llm_cache"
	defaultThreshold  = 0.97
	defaultCacheTTL   = 1 * time.Hour
	embeddingModel    = "sentence-transformers/all-MiniLM-L6-v2"
	embeddingOnnxPath = "onnx/model.onnx"
	modelDir          = "./models/"
)

type SemanticCache struct {
	rdb       *redis.Client
	pipeline  *pipelines.FeatureExtractionPipeline
	session   *hugot.Session
	threshold float64
	vecSetKey string
}

func NewSemanticCache(rdb *redis.Client, threshold float64) (*SemanticCache, error) {
	session, err := hugot.NewGoSession()
	if err != nil {
		return nil, fmt.Errorf("hugot session: %w", err)
	}

	downloadOptions := hugot.NewDownloadOptions()
	downloadOptions.OnnxFilePath = embeddingOnnxPath
	modelPath, err := hugot.DownloadModel(embeddingModel, modelDir, downloadOptions)
	if err != nil {
		session.Destroy()
		return nil, fmt.Errorf("download model: %w", err)
	}

	config := hugot.FeatureExtractionConfig{
		ModelPath: modelPath,
		Name:      "semanticCache",
	}
	pipeline, err := hugot.NewPipeline(session, config)
	if err != nil {
		session.Destroy()
		return nil, fmt.Errorf("create pipeline: %w", err)
	}

	return &SemanticCache{
		rdb:       rdb,
		pipeline:  pipeline,
		session:   session,
		threshold: threshold,
		vecSetKey: defaultVecSetKey,
	}, nil
}

func (c *SemanticCache) embed(text string) ([]float64, error) {
	result, err := c.pipeline.RunPipeline([]string{text})
	if err != nil {
		return nil, err
	}
	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	f32 := result.Embeddings[0]
	f64 := make([]float64, len(f32))
	for i, v := range f32 {
		f64[i] = float64(v)
	}
	return f64, nil
}

func (c *SemanticCache) Lookup(queryText string) (*LLMResponse, bool) {
	ctx := context.Background()

	embedding, err := c.embed(queryText)
	if err != nil {
		log.Printf("[cache] embed error: %v", err)
		return nil, false
	}

	results, err := c.rdb.VSimWithArgsWithScores(ctx, c.vecSetKey,
		&redis.VectorValues{Val: embedding},
		&redis.VSimArgs{Count: 1},
	).Result()
	if err != nil {
		log.Printf("[cache] vsim error: %v", err)
		return nil, false
	}

	if len(results) == 0 || results[0].Score < c.threshold {
		return nil, false
	}

	attrJSON, err := c.rdb.VGetAttr(ctx, c.vecSetKey, results[0].Name).Result()
	if err != nil {
		log.Printf("[cache] vgetattr error: %v", err)
		return nil, false
	}

	var resp LLMResponse
	if err := json.Unmarshal([]byte(attrJSON), &resp); err != nil {
		log.Printf("[cache] unmarshal error: %v", err)
		return nil, false
	}

	log.Printf("[cache] HIT score=%.4f key=%s", results[0].Score, results[0].Name)
	return &resp, true
}

func (c *SemanticCache) Store(queryText string, resp *LLMResponse) {
	ctx := context.Background()

	embedding, err := c.embed(queryText)
	if err != nil {
		log.Printf("[cache] embed error on store: %v", err)
		return
	}

	cacheKey := hashQuery(queryText)

	_, err = c.rdb.VAdd(ctx, c.vecSetKey, cacheKey, &redis.VectorValues{Val: embedding}).Result()
	if err != nil {
		log.Printf("[cache] vadd error: %v", err)
		return
	}

	respJSON, err := json.Marshal(resp)
	if err != nil {
		log.Printf("[cache] marshal error: %v", err)
		return
	}

	_, err = c.rdb.VSetAttr(ctx, c.vecSetKey, cacheKey, string(respJSON)).Result()
	if err != nil {
		log.Printf("[cache] vsetattr error: %v", err)
		return
	}

	// refresh TTL on the entire VecSet (VecSets don't support per-element TTL)
	c.rdb.Expire(ctx, c.vecSetKey, defaultCacheTTL)

	log.Printf("[cache] STORED key=%s ttl=%v", cacheKey, defaultCacheTTL)
}

func (c *SemanticCache) Close() {
	if c.session != nil {
		c.session.Destroy()
	}
}

func extractQueryText(messages []Message) string {
	var parts []string
	for _, m := range messages {
		if m.Role == "user" {
			if text := m.GetContentString(); text != "" {
				parts = append(parts, text)
			}
		}
	}

	return strings.Join(parts, "\n")
}

func hashQuery(query string) string {
	h := sha256.Sum256([]byte(query))
	return hex.EncodeToString(h[:])
}
