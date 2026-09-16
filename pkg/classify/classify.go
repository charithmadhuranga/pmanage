package classify

import (
	"path/filepath"
	"strings"
)

// Kind groups processes into workload buckets.
type Kind string

const (
	KindTraining  Kind = "training"
	KindInference Kind = "inference"
	KindCodec     Kind = "codec"
	KindCrypto    Kind = "crypto"
	KindUnknown   Kind = "unknown"
)

// RuleHit records one matching classification rule: why the process was
// assigned a bucket. Provenance = rule name; Weight = confidence-ish score.
type RuleHit struct {
	Rule   string `json:"rule"`
	Signal string `json:"signal"` // the matched token/path
	Score  float64 `json:"score"` // additive confidence contribution
}

// Classified is the machine-readable result for one process.
type Classified struct {
	PID   int32     `json:"pid"`
	Kind  Kind      `json:"kind"`
	Score float64   `json:"score"` // 0..1 aggregate confidence
	Hits  []RuleHit `json:"hits"`
}

// Signals carries the observable evidence available for classification:
// executable name/path, command line, environment, and loaded libraries
// (extracted from open file handles / dlopen'd .so names).
type Signals struct {
	PID     int32
	Name    string // process name (basename)
	Exe     string // resolved executable path
	Cmdline string // full command line
	Env     []string
	Libs    []string // loaded shared-library basenames
}

// Classify buckets a process from its signals. Pure function, testable with
// fixtures; deterministic (no fs access).
func Classify(s Signals) Classified {
	// Collection of candidate signals for fast substring checks.
	text := s.Name + " " + s.Exe + " " + s.Cmdline
	lower := strings.ToLower(text)
	libSet := map[string]bool{}
	for _, l := range s.Libs {
		libSet[l] = true
	}
	envSet := map[string]bool{}
	for _, e := range s.Env {
		envSet[e] = true
	}

	var hits []RuleHit
	var score float64
	kind := KindUnknown

	// --- Crypto first (most specific signal, can co-mine) ---
	crypto := cryptoHits(lower, s.Name, libSet)
	if len(crypto) > 0 {
		kind = KindCrypto
		score = 1.0
		hits = crypto
		return Classified{PID: s.PID, Kind: kind, Score: score, Hits: hits}
	}

	// --- Training ---
	if t := trainingHits(lower, s.Name, envSet, libSet); len(t) > 0 {
		hits = append(hits, t...)
		score += ruleScore(t)
		if score >= 0.3 {
			kind = KindTraining
			return Classified{PID: s.PID, Kind: kind, Score: score, Hits: hits}
		}
	}

	// --- Inference ---
	if inf := inferenceHits(lower, s.Name, libSet); len(inf) > 0 {
		hits = append(hits, inf...)
		score += ruleScore(inf)
		if score >= 0.3 {
			kind = KindInference
			return Classified{PID: s.PID, Kind: kind, Score: score, Hits: hits}
		}
	}

	// --- Codec ---
	if c := codecHits(lower, s.Name, libSet); len(c) > 0 {
		hits = append(hits, c...)
		score += ruleScore(c)
		if score >= 0.3 {
			kind = KindCodec
			return Classified{PID: s.PID, Kind: kind, Score: score, Hits: hits}
		}
	}

	return Classified{PID: s.PID, Kind: kind, Score: score, Hits: hits}
}

func ruleScore(hits []RuleHit) float64 {
	var total float64
	for _, h := range hits {
		total += h.Score
	}
	if total > 1.0 {
		return 1.0
	}
	return total
}

func matchHit(rule, signal string, score float64) RuleHit {
	return RuleHit{Rule: rule, Signal: signal, Score: score}
}

func cryptoHits(lower, name string, _ map[string]bool) []RuleHit {
	var hits []RuleHit
	markers := []struct{ rule, token string }{
		{"name:xmrig", "xmrig"}, {"name:miner", "miner"},
		{"name:ethminer", "ethminer"}, {"name:lolminer", "lolminer"},
		{"name:teamredminer", "teamredminer"}, {"name:gminer", "gminer"},
		{"name:phoenixminer", "phoenixminer"}, {"name:csminer", "csminer"},
		{"name:trex", "trex"}, {"name:wildrig", "wildrig"},
		{"name:ccminer", "ccminer"}, {"name:kbminer", "kbminer"},
		{"name:nsfminer", "nsfminer"}, {"name:bminer", "bminer"},
		{"name:cryptodredge", "cryptodredge"},
		{"cmd:pool", "pool"}, {"cmd:stratum", "stratum"},
		{"cmd:wallet", "wallet"},
	}
	for _, m := range markers {
		if strings.Contains(lower, m.token) {
			hits = append(hits, matchHit("crypto:"+m.rule, m.token, 0.8))
		}
	}
	return hits
}
func trainingHits(lower, _ string, envSet, libSet map[string]bool) []RuleHit {
	var hits []RuleHit
	libs := []struct{ rule, lib string }{
		{"lib:cudnn", "libcudnn.so"}, {"lib:cublas", "libcublas.so"},
		{"lib:deepspeed", "deepspeed"}, {"lib:oneapi", "libonnxruntime"}, // shared below
		{"lib:torch", "libtorch"}, {"lib:tensorflow", "libtensorflow"},
		{"lib:kleidi", "libkleidi"},
	}
	for _, l := range libs {
		if hasLib(libSet, l.lib) {
			hits = append(hits, matchHit(l.rule, l.lib, 0.6))
		}
	}
	if envSet["PYTORCH_CUDA_ALLOC_CONF"] || envSet["CUDA_CACHE_DISABLE"] {
		hits = append(hits, matchHit("env:ml", "CUDA_* env", 0.3))
	}
	for _, tok := range []string{"train", "finetune", "ft-", "tune.py"} {
		if strings.Contains(lower, tok) {
			hits = append(hits, matchHit("cmd:"+tok, tok, 0.7))
			break
		}
	}
	return hits
}
func inferenceHits(lower, _ string, libSet map[string]bool) []RuleHit {
	var hits []RuleHit
	libs := []struct{ rule, lib string }{
		{"lib:onnxruntime", "onnxruntime"}, {"lib:libonnxruntime", "libonnxruntime.so"},
		{"lib:tensorrt", "libnvinfer.so"}, {"lib:openvino", "openvino"},
		{"lib:vllm", "vllm"}, {"lib:llama", "llama"},
		{"lib:libedgetpu", "libedgetpu.so"}, {"lib:libhailort", "libhailort.so"},
		{"lib:tflite", "tflite"}, {"lib:torchserve", "torchserve"},
	}
	for _, l := range libs {
		if hasLib(libSet, l.lib) {
			hits = append(hits, matchHit(l.rule, l.lib, 0.6))
		}
	}
	for _, tok := range []string{"infer", "serve", "generate", "enc-dec", "llm"} {
		if strings.Contains(lower, tok) {
			hits = append(hits, matchHit("cmd:"+tok, tok, 0.25))
		}
	}
	return hits
}
func codecHits(lower, _ string, libSet map[string]bool) []RuleHit {
	var hits []RuleHit
	libs := []struct{ rule, lib string }{
		{"lib:libavcodec", "libavcodec"}, {"lib:libav", "libav"},
		{"lib:x264", "x264"}, {"lib:x265", "x265"}, {"lib:nvcodec", "nvidia-encode"},
		{"lib:vp9", "vp9"}, {"lib:aom", "libaom"}, {"lib:ffmpeg", "ffmpeg"},
		{"lib:gstreamer", "gstreamer"},
	}
	for _, l := range libs {
		if hasLib(libSet, l.lib) {
			hits = append(hits, matchHit(l.rule, l.lib, 0.5))
		}
	}
	for _, tok := range []string{"ffmpeg", "ffprobe", "gst-launch", "transcode", "-c:v", "h264"} {
		if strings.Contains(lower, tok) {
			hits = append(hits, matchHit("cmd:"+tok, tok, 0.4))
		}
	}
	return hits
}

func hasLib(libSet map[string]bool, needle string) bool {
	if libSet[needle] {
		return true
	}
	base := filepath.Base(needle)
	for l := range libSet {
		if l == needle || base != "" && strings.Contains(l, base) {
			return true
		}
	}
	return false
}