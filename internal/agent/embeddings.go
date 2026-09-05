package agent

import (
	"math"
	"sort"
	"strings"

	"github.com/Rishi0507/manhattan/internal/evidence"
)

// DocumentVector represents a receipt as a TF-IDF vector for semantic search.
type DocumentVector struct {
	ReceiptID string
	Vector    map[string]float64
	Norm      float64
}

// VectorStore holds pre-computed embeddings for all receipts.
type VectorStore struct {
	docs []DocumentVector
	idf  map[string]float64 // inverse document frequency
}

// BuildVectorStore creates embeddings for all receipts using TF-IDF.
// This is a deterministic, fast alternative to neural embeddings.
func BuildVectorStore(receipts []*evidence.Receipt) *VectorStore {
	vs := &VectorStore{
		docs: make([]DocumentVector, 0, len(receipts)),
		idf:  make(map[string]float64),
	}

	// Build document frequency map
	df := make(map[string]int)
	for _, r := range receipts {
		terms := vs.extractTerms(r)
		seen := make(map[string]bool)
		for term := range terms {
			if !seen[term] {
				df[term]++
				seen[term] = true
			}
		}
	}

	// Calculate IDF
	n := float64(len(receipts))
	for term, freq := range df {
		vs.idf[term] = math.Log(n / float64(freq))
	}

	// Build TF-IDF vectors for each document
	for _, r := range receipts {
		terms := vs.extractTerms(r)
		vector := make(map[string]float64)
		
		// Calculate term frequency
		maxFreq := 0.0
		for _, freq := range terms {
			if float64(freq) > maxFreq {
				maxFreq = float64(freq)
			}
		}

		// TF-IDF calculation
		var norm float64
		for term, freq := range terms {
			tf := float64(freq) / maxFreq
			tfidf := tf * vs.idf[term]
			vector[term] = tfidf
			norm += tfidf * tfidf
		}
		norm = math.Sqrt(norm)

		vs.docs = append(vs.docs, DocumentVector{
			ReceiptID: r.SettlementRef,
			Vector:    vector,
			Norm:      norm,
		})
	}

	return vs
}

// extractTerms extracts searchable terms from a receipt with frequencies.
func (vs *VectorStore) extractTerms(r *evidence.Receipt) map[string]int {
	terms := make(map[string]int)
	
	// Extract from settlement reference
	for _, token := range tokenize(r.SettlementRef) {
		terms[token]++
	}
	
	// Extract from status (weighted higher)
	statusTerms := tokenize(string(r.Status))
	for _, token := range statusTerms {
		terms[token] += 3 // Status is important
	}
	
	// Extract from merchant and archetype
	for _, token := range tokenize(r.MerchantName) {
		terms[token] += 2
	}
	for _, token := range tokenize(r.Archetype) {
		terms[token] += 2
	}
	
	// Extract from flags
	for _, flag := range r.Flags {
		for _, token := range tokenize(string(flag)) {
			terms[token] += 2
		}
	}
	
	// Extract from remediation actions
	for _, rem := range r.Remediation {
		for _, token := range tokenize(rem.Action) {
			terms[token]++
		}
	}
	
	// Semantic field indicators
	if r.Agent.Invoked {
		terms["agent"] += 3
		terms["repair"] += 3
		terms["action"] += 2
	}
	if r.AmountEntropy.TwinMass > 0.3 {
		terms["twin"] += 2
		terms["collision"] += 2
	}
	if r.ExceptionCostINR > 1000 {
		terms["expensive"] += 2
		terms["costly"] += 2
		terms["high"] += 1
	}
	if r.FeeCheck != nil && r.FeeCheck.Circular {
		terms["circular"] += 3
		terms["fee"] += 2
	}
	
	return terms
}

// Search performs cosine similarity search against query.
func (vs *VectorStore) Search(query string, k int) []string {
	// Build query vector
	queryTerms := make(map[string]int)
	for _, token := range tokenize(query) {
		queryTerms[token]++
	}
	
	queryVec := make(map[string]float64)
	var queryNorm float64
	maxFreq := 0
	for _, freq := range queryTerms {
		if freq > maxFreq {
			maxFreq = freq
		}
	}
	
	for term, freq := range queryTerms {
		tf := float64(freq) / float64(maxFreq)
		tfidf := tf * vs.idf[term]
		queryVec[term] = tfidf
		queryNorm += tfidf * tfidf
	}
	queryNorm = math.Sqrt(queryNorm)
	
	// Calculate cosine similarity for each document
	type scored struct {
		id    string
		score float64
	}
	scores := make([]scored, 0, len(vs.docs))
	
	for _, doc := range vs.docs {
		if queryNorm == 0 || doc.Norm == 0 {
			scores = append(scores, scored{doc.ReceiptID, 0})
			continue
		}
		
		// Dot product
		var dot float64
		for term, qVal := range queryVec {
			if dVal, ok := doc.Vector[term]; ok {
				dot += qVal * dVal
			}
		}
		
		// Cosine similarity
		similarity := dot / (queryNorm * doc.Norm)
		scores = append(scores, scored{doc.ReceiptID, similarity})
	}
	
	// Sort by score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})
	
	// Return top k
	if k > len(scores) {
		k = len(scores)
	}
	result := make([]string, k)
	for i := 0; i < k; i++ {
		result[i] = scores[i].id
	}
	
	return result
}

// tokenize splits text into normalized tokens.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	// Replace underscores and dashes with spaces
	text = strings.ReplaceAll(text, "_", " ")
	text = strings.ReplaceAll(text, "-", " ")
	
	// Split on whitespace
	tokens := strings.Fields(text)
	
	// Filter short tokens
	filtered := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if len(token) >= 2 {
			filtered = append(filtered, token)
		}
	}
	
	return filtered
}
