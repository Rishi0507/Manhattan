# Manhattan, without make. See run.sh for the POSIX equivalent.
#
#   .\run.ps1 demo     build, benchmark, serve the dashboard
#   .\run.ps1 bench    benchmark and regenerate RESULTS.md
#   .\run.ps1 cases    the eleven adversarial cases, head to head
#   .\run.ps1 test     the full test suite
param([string]$Task = "demo", [int]$N = 500, [long]$Seed = 20260826, [string]$Addr = ":8080")

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$Bin = "bin/manhattan.exe"

function Build-Web {
  if (Get-Command npm -ErrorAction SilentlyContinue) {
    Push-Location web; npm install --silent; npm run build; Pop-Location
  } else {
    Write-Host "npm not found, serving the previously built dashboard"
  }
}
function Build-Go { go build -trimpath -o $Bin ./cmd/manhattan }

switch ($Task) {
  "demo" {
    Build-Web; Build-Go
    & $Bin bench -n $N -seed $Seed -out out
    Write-Host ""
    Write-Host "  Dashboard on http://localhost$Addr. Start at the head-to-head tab."
    Write-Host ""
    & $Bin serve -addr $Addr -store out
  }
  "bench" { Build-Go; & $Bin bench -n $N -seed $Seed -out out }
  "cases" { Build-Go; & $Bin cases -out out }
  "recon" { Build-Go; & $Bin recon -n 12 -archetype travel }
  "serve" { Build-Go; & $Bin serve -addr $Addr -store out }
  "web"   { Build-Web }
  "test"  { go test ./... -count=1 }
  "perf"  { go test ./internal/solver/ -run TestPerformanceGate -v -count=1 }
  default { Write-Host "usage: .\run.ps1 [demo|bench|cases|recon|serve|web|test|perf]"; exit 2 }
}
