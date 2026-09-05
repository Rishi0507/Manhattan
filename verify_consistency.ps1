# Manhattan Project - Consistency Verification Script

Write-Host "=== MANHATTAN CONSISTENCY CHECK ===" -ForegroundColor Cyan
Write-Host ""

$errors = 0

# Check 1: Verify vector embeddings file exists
Write-Host "✓ Checking vector embeddings implementation..." -ForegroundColor Yellow
if (Test-Path "internal/agent/embeddings.go") {
    Write-Host "  ✅ internal/agent/embeddings.go exists" -ForegroundColor Green
} else {
    Write-Host "  ❌ internal/agent/embeddings.go missing" -ForegroundColor Red
    $errors++
}

# Check 2: Verify knowledge graph file exists
Write-Host "✓ Checking knowledge graph implementation..." -ForegroundColor Yellow
if (Test-Path "internal/agent/knowledge_graph.go") {
    Write-Host "  ✅ internal/agent/knowledge_graph.go exists" -ForegroundColor Green
} else {
    Write-Host "  ❌ internal/agent/knowledge_graph.go missing" -ForegroundColor Red
    $errors++
}

# Check 3: Verify Q&A integration
Write-Host "✓ Checking Q&A agent integration..." -ForegroundColor Yellow
$qaContent = Get-Content "internal/agent/qa.go" -Raw
if ($qaContent -match "VectorStore \*VectorStore") {
    Write-Host "  ✅ Q&A has VectorStore field" -ForegroundColor Green
} else {
    Write-Host "  ❌ Q&A missing VectorStore field" -ForegroundColor Red
    $errors++
}
if ($qaContent -match "Graph \*KnowledgeGraph") {
    Write-Host "  ✅ Q&A has Graph field" -ForegroundColor Green
} else {
    Write-Host "  ❌ Q&A missing Graph field" -ForegroundColor Red
    $errors++
}
if ($qaContent -match "BuildVectorStore") {
    Write-Host "  ✅ Q&A builds vector store" -ForegroundColor Green
} else {
    Write-Host "  ❌ Q&A doesn't build vector store" -ForegroundColor Red
    $errors++
}

# Check 4: Verify benchmark integration
Write-Host "✓ Checking benchmark integration..." -ForegroundColor Yellow
$benchContent = Get-Content "internal/bench/run.go" -Raw
if ($benchContent -match "forecast\.New") {
    Write-Host "  ✅ Benchmark uses forecast" -ForegroundColor Green
} else {
    Write-Host "  ❌ Benchmark missing forecast" -ForegroundColor Red
    $errors++
}
if ($benchContent -match "taxmatch\.New") {
    Write-Host "  ✅ Benchmark uses tax matcher" -ForegroundColor Green
} else {
    Write-Host "  ❌ Benchmark missing tax matcher" -ForegroundColor Red
    $errors++
}

# Check 5: Verify documentation exists
Write-Host "✓ Checking documentation..." -ForegroundColor Yellow
$docs = @(
    "README.md",
    "FEATURES_ADDED.md", 
    "VECTOR_GRAPH_RAG_IMPLEMENTATION.md",
    "docs/diagrams/knowledge-graph.mmd"
)
foreach ($doc in $docs) {
    if (Test-Path $doc) {
        Write-Host "  ✅ $doc exists" -ForegroundColor Green
    } else {
        Write-Host "  ❌ $doc missing" -ForegroundColor Red
        $errors++
    }
}

# Check 6: Verify no "fake" language in docs (except meta-docs about removal)
Write-Host "✓ Checking for inappropriate 'fake' language..." -ForegroundColor Yellow
$problematicFiles = @("README.md", "internal/agent/qa.go", "internal/server/server.go", "web/src/Ask.tsx")
$foundProblematic = $false
foreach ($file in $problematicFiles) {
    if (Test-Path $file) {
        $content = Get-Content $file -Raw
        if ($content -match "fake loading|Fake loading|fake animation") {
            Write-Host "  ⚠️  Found 'fake' language in $file" -ForegroundColor Yellow
            $foundProblematic = $true
        }
    }
}
if (-not $foundProblematic) {
    Write-Host "  ✅ No inappropriate 'fake' language found" -ForegroundColor Green
}

# Check 7: Verify build
Write-Host "✓ Checking build..." -ForegroundColor Yellow
if (Test-Path "bin/manhattan.exe") {
    Write-Host "  ✅ Binary exists" -ForegroundColor Green
} else {
    Write-Host "  ⚠️  Binary not found (run: go build -o bin/manhattan.exe ./cmd/manhattan)" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "=== CONSISTENCY CHECK COMPLETE ===" -ForegroundColor Cyan
if ($errors -eq 0) {
    Write-Host "✅ All checks passed! System is consistent." -ForegroundColor Green
} else {
    Write-Host "❌ Found $errors error(s). Please review." -ForegroundColor Red
}
