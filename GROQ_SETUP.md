# Groq Setup Guide

## Why Groq?

Groq is recommended over Gemini for the AI buildathon because:
- ✅ **Free tier:** 30 requests/minute, 14,400/day (won't exhaust quota)
- ✅ **Blazing fast:** 500+ tokens/sec (10x faster than typical APIs)
- ✅ **No credit card:** Completely free to start
- ✅ **Reliable:** Purpose-built for high-throughput inference

## Quick Setup (2 minutes)

### 1. Get Your API Key

1. Go to https://console.groq.com/keys
2. Sign up with Google/GitHub (takes 30 seconds)
3. Click "Create API Key"
4. Copy the key (starts with `gsk_...`)

### 2. Add to .env File

Open `.env` in the project root and add:

```bash
GROQ_API_KEY=gsk_your_key_here
```

### 3. Run Live API Test

```bash
# Run with just 20 settlements to start
./bin/manhattan.exe live -n 20 -provider groq

# If that works, try 60 settlements for full results
./bin/manhattan.exe live -n 60 -provider groq
```

## What This Gives You

Running `manhattan live` with Groq will generate:
- **Real AI metrics** (not stub/replay)
- **Diagnosis accuracy delta** (Groq vs stub)
- **Agent repair count** with real model reasoning
- **Actual billed costs** (not estimated)
- **Proof that AI integration works**

The command writes `out/live.json` with the delta between live API and offline stub.

## Expected Results

For 20 settlements, expect:
- ~100-200 API calls
- ~1-2 minutes runtime (Groq is FAST)
- Diagnosis accuracy improvement: 65% (stub) → 75-85% (Groq)
- Cost: ~₹20-40 total

For 60 settlements:
- ~400-600 API calls  
- ~3-5 minutes runtime
- Full comparison data for README
- Cost: ~₹60-120 total

## Troubleshooting

**"Rate limit exceeded"**
- Wait 1 minute and try again
- Or reduce `-n 20` to `-n 10`

**"API key not set"**
- Check `.env` file has `GROQ_API_KEY=gsk_...`
- No quotes needed around the key
- Make sure there's no space before the key

**"Model not found"**
- Check you're using latest binary: `go build -trimpath -o bin/manhattan.exe ./cmd/manhattan`

## Available Groq Models

The system uses:
- **llama-3.3-70b-versatile** - For complex reasoning (plan, resolve, control)
- **llama-3.1-8b-instant** - For fast tasks (parse, triage, notes)

You can override with environment variables:
```bash
GROQ_MODEL=llama-3.3-70b-versatile  # For quality
# or
GROQ_MODEL=mixtral-8x7b-32768      # For speed
```

## Next Steps After Running

1. Check `out/live.json` was created
2. Run `./bin/manhattan.exe docs` to regenerate README with live results
3. Compare diagnosis accuracy: stub vs Groq
4. Use these real numbers in your presentation!

## Cost Comparison

| Provider | Free Tier | Quota Issues | Speed | Recommended? |
|----------|-----------|--------------|-------|--------------|
| **Groq** | 14,400/day | ❌ Rare | ⚡ 500 tok/s | ✅ **YES** |
| Gemini | 10/min | ✅ Common | 🐌 50 tok/s | ⚠️ Risky |
| Anthropic | $5 credits | ❌ No (paid) | 🐌 50 tok/s | ✅ If paid |

---

**Ready?** Just add your `GROQ_API_KEY` to `.env` and run:
```bash
./bin/manhattan.exe live -n 20 -provider groq
```
