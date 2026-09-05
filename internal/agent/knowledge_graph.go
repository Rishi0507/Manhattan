package agent

import (
	"fmt"
	"strings"

	"github.com/Rishi0507/manhattan/internal/evidence"
)

// KnowledgeGraph represents relationships between settlements for Graph RAG.
type KnowledgeGraph struct {
	nodes map[string]*Node
	edges []*Edge
}

// Node represents a settlement in the knowledge graph.
type Node struct {
	ID          string
	Type        string // "settlement", "merchant", "status", "flag"
	Properties  map[string]any
	Neighbors   []*Edge
}

// Edge represents a relationship between nodes.
type Edge struct {
	From         string
	To           string
	Relationship string // "same_merchant", "same_status", "caused_by", "remedied_by"
	Weight       float64
}

// BuildKnowledgeGraph constructs a graph from receipts for multi-hop reasoning.
func BuildKnowledgeGraph(receipts []*evidence.Receipt) *KnowledgeGraph {
	kg := &KnowledgeGraph{
		nodes: make(map[string]*Node),
		edges: make([]*Edge, 0),
	}

	// Create settlement nodes
	for _, r := range receipts {
		node := &Node{
			ID:   r.SettlementRef,
			Type: "settlement",
			Properties: map[string]any{
				"status":           r.Status,
				"merchant":         r.MerchantName,
				"archetype":        r.Archetype,
				"target_paise":     r.TargetPaise,
				"exception_cost":   r.ExceptionCostINR,
				"agent_invoked":    r.Agent.Invoked,
				"flags":            r.Flags,
			},
			Neighbors: make([]*Edge, 0),
		}
		kg.nodes[r.SettlementRef] = node
	}

	// Create merchant nodes
	merchantNodes := make(map[string]*Node)
	for _, r := range receipts {
		if r.MerchantName == "" {
			continue
		}
		if _, exists := merchantNodes[r.MerchantName]; !exists {
			merchantNodes[r.MerchantName] = &Node{
				ID:   "merchant:" + r.MerchantName,
				Type: "merchant",
				Properties: map[string]any{
					"name":      r.MerchantName,
					"archetype": r.Archetype,
				},
				Neighbors: make([]*Edge, 0),
			}
		}
	}
	for _, node := range merchantNodes {
		kg.nodes[node.ID] = node
	}

	// Create status nodes
	statusNodes := make(map[evidence.Status]*Node)
	for _, r := range receipts {
		if _, exists := statusNodes[r.Status]; !exists {
			statusNodes[r.Status] = &Node{
				ID:   "status:" + string(r.Status),
				Type: "status",
				Properties: map[string]any{
					"status":   r.Status,
					"postable": r.Status.Postable(),
				},
				Neighbors: make([]*Edge, 0),
			}
		}
	}
	for _, node := range statusNodes {
		kg.nodes[node.ID] = node
	}

	// Create edges: settlement -> merchant
	for _, r := range receipts {
		if r.MerchantName == "" {
			continue
		}
		edge := &Edge{
			From:         r.SettlementRef,
			To:           "merchant:" + r.MerchantName,
			Relationship: "belongs_to",
			Weight:       1.0,
		}
		kg.edges = append(kg.edges, edge)
		kg.nodes[r.SettlementRef].Neighbors = append(kg.nodes[r.SettlementRef].Neighbors, edge)
	}

	// Create edges: settlement -> status
	for _, r := range receipts {
		edge := &Edge{
			From:         r.SettlementRef,
			To:           "status:" + string(r.Status),
			Relationship: "has_status",
			Weight:       1.0,
		}
		kg.edges = append(kg.edges, edge)
		kg.nodes[r.SettlementRef].Neighbors = append(kg.nodes[r.SettlementRef].Neighbors, edge)
	}

	// Create edges: settlements with same root cause
	causeGroups := make(map[string][]string)
	for _, r := range receipts {
		if len(r.Remediation) == 0 {
			continue
		}
		cause := r.Remediation[0].Action // Primary remediation indicates root cause
		causeGroups[cause] = append(causeGroups[cause], r.SettlementRef)
	}

	for cause, group := range causeGroups {
		if len(group) < 2 {
			continue
		}
		// Connect settlements with same root cause
		for i := 0; i < len(group); i++ {
			for j := i + 1; j < len(group); j++ {
				edge := &Edge{
					From:         group[i],
					To:           group[j],
					Relationship: "same_root_cause:" + cause,
					Weight:       0.8,
				}
				kg.edges = append(kg.edges, edge)
				kg.nodes[group[i]].Neighbors = append(kg.nodes[group[i]].Neighbors, edge)
			}
		}
	}

	// Create edges: agent repairs
	for _, r := range receipts {
		if !r.Agent.Invoked || r.Agent.Accepted == nil {
			continue
		}
		// This settlement was repaired by agent
		edge := &Edge{
			From:         r.SettlementRef,
			To:           "action:" + string(r.Agent.Accepted.Kind),
			Relationship: "repaired_by",
			Weight:       1.0,
		}
		kg.edges = append(kg.edges, edge)
	}

	return kg
}

// TraverseGraph performs multi-hop reasoning from a query node.
func (kg *KnowledgeGraph) TraverseGraph(startID string, maxHops int) []string {
	visited := make(map[string]bool)
	result := []string{startID}
	visited[startID] = true

	current := []string{startID}
	for hop := 0; hop < maxHops; hop++ {
		next := []string{}
		for _, nodeID := range current {
			node, exists := kg.nodes[nodeID]
			if !exists {
				continue
			}
			for _, edge := range node.Neighbors {
				if !visited[edge.To] {
					visited[edge.To] = true
					result = append(result, edge.To)
					next = append(next, edge.To)
				}
			}
		}
		if len(next) == 0 {
			break
		}
		current = next
	}

	return result
}

// FindPath finds shortest path between two nodes (for causal reasoning).
func (kg *KnowledgeGraph) FindPath(fromID, toID string) []string {
	if fromID == toID {
		return []string{fromID}
	}

	visited := make(map[string]bool)
	queue := [][]string{{fromID}}
	visited[fromID] = true

	for len(queue) > 0 {
		path := queue[0]
		queue = queue[1:]
		
		current := path[len(path)-1]
		node, exists := kg.nodes[current]
		if !exists {
			continue
		}

		for _, edge := range node.Neighbors {
			if edge.To == toID {
				return append(path, toID)
			}
			if !visited[edge.To] {
				visited[edge.To] = true
				newPath := make([]string, len(path))
				copy(newPath, path)
				newPath = append(newPath, edge.To)
				queue = append(queue, newPath)
			}
		}
	}

	return nil // No path found
}

// ExplainRelationship describes how two settlements are related.
func (kg *KnowledgeGraph) ExplainRelationship(id1, id2 string) string {
	path := kg.FindPath(id1, id2)
	if path == nil {
		return "No direct relationship found"
	}

	if len(path) == 2 {
		// Direct connection
		node1 := kg.nodes[id1]
		for _, edge := range node1.Neighbors {
			if edge.To == id2 {
				return fmt.Sprintf("Directly connected via %s", edge.Relationship)
			}
		}
	}

	// Multi-hop explanation
	parts := []string{}
	for i := 0; i < len(path)-1; i++ {
		node := kg.nodes[path[i]]
		for _, edge := range node.Neighbors {
			if edge.To == path[i+1] {
				parts = append(parts, edge.Relationship)
				break
			}
		}
	}

	return fmt.Sprintf("Connected via %d hops: %s", len(path)-1, strings.Join(parts, " → "))
}

// FindSimilarSettlements uses graph structure to find related settlements.
func (kg *KnowledgeGraph) FindSimilarSettlements(settlementID string, limit int) []string {
	neighbors := kg.TraverseGraph(settlementID, 2) // 2-hop neighbors
	
	// Score by graph distance and relationship type
	type scored struct {
		id    string
		score float64
	}
	scores := make([]scored, 0)
	
	for _, nid := range neighbors {
		if nid == settlementID || !strings.HasPrefix(nid, "bank_credit_") {
			continue // Skip self and non-settlement nodes
		}
		
		// Calculate score based on shared connections
		node1 := kg.nodes[settlementID]
		node2 := kg.nodes[nid]
		
		commonNeighbors := 0.0
		for _, e1 := range node1.Neighbors {
			for _, e2 := range node2.Neighbors {
				if e1.To == e2.To {
					commonNeighbors += 1.0
				}
			}
		}
		
		score := commonNeighbors / float64(len(node1.Neighbors)+1)
		scores = append(scores, scored{nid, score})
	}
	
	// Sort by score
	for i := 0; i < len(scores); i++ {
		for j := i + 1; j < len(scores); j++ {
			if scores[j].score > scores[i].score {
				scores[i], scores[j] = scores[j], scores[i]
			}
		}
	}
	
	// Return top k
	if limit > len(scores) {
		limit = len(scores)
	}
	result := make([]string, limit)
	for i := 0; i < limit; i++ {
		result[i] = scores[i].id
	}
	
	return result
}
