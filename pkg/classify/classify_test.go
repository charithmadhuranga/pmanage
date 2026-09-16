package classify

import (
	"testing"
)

func mustKind(t *testing.T, sig Signals, want Kind) Classified {
	t.Helper()
	c := Classify(sig)
	if c.Kind != want {
		t.Errorf("expected %q, got %q (hits=%+v)", want, c.Kind, c.Hits)
	}
	if c.Score <= 0 && c.Kind != KindUnknown {
		t.Errorf("expected positive score for %q", c.Kind)
	}
	return c
}

func TestClassifyCrypto(t *testing.T) {
	c := mustKind(t, Signals{
		Name:    "xmrig",
		Cmdline: "xmrig --cuda -o pool.minexmr.com:4444",
	}, KindCrypto)
	if c.Score < 0.8 {
		t.Errorf("crypto should be high confidence, got %f", c.Score)
	}
	if len(c.Hits) < 2 {
		t.Errorf("expected name + pool hits, got %+v", c.Hits)
	}
}

func TestClassifyTrainingOnnxWorkout(t *testing.T) {
	// onnxruntime + trn script → training bucket via deepspeed-ish signals
	c := mustKind(t, Signals{
		Name:    "python3",
		Exe:     "/usr/bin/python3",
		Cmdline: "python3 train.py --epochs 10 --gpu",
		Env:     []string{"CUDA_VISIBLE_DEVICES=0", "PYTORCH_CUDA_ALLOC_CONF=expandable_segments"},
		Libs:    []string{"libcudnn.so", "libcublas.so"},
	}, KindTraining)
	if c.Score < 0.5 {
		t.Errorf("expected decent training confidence, got %f", c.Score)
	}
}

func TestClassifyInferenceVLLM(t *testing.T) {
	mustKind(t, Signals{
		Name:    "vllm",
		Cmdline: "vllm serve meta-llama/Llama-3-8B --tensor-parallel-size 2",
		Libs:    []string{"libnvinfer.so", "vllm"},
	}, KindInference)
}

func TestClassifyCodecFFmpeg(t *testing.T) {
	mustKind(t, Signals{
		Name:    "ffmpeg",
		Cmdline: "ffmpeg -c:v h264_nvenc -i input.mp4 out.mp4",
		Libs:    []string{"libavcodec.so", "libnvidia-encode.so"},
	}, KindCodec)
}

func TestClassifyUnknown(t *testing.T) {
	c := mustKind(t, Signals{Name: "ls", Cmdline: "ls -la"}, KindUnknown)
	if len(c.Hits) != 0 {
		t.Errorf("expected no hits for unknown, got %+v", c.Hits)
	}
}

func TestClassifyCryptoBeatsTraining(t *testing.T) {
	// a miner binary mislabeled with a train-ish flag must still be crypto
	c := Classify(Signals{Name: "xmrig", Cmdline: "xmrig --train-wallet-opt"})
	if c.Kind != KindCrypto {
		t.Errorf("crypto must take precedence, got %q", c.Kind)
	}
}

func TestHasLibFuzzy(t *testing.T) {
	if !hasLib(map[string]bool{"libonnxruntime.so.1.17.1": true}, "libonnxruntime.so") {
		t.Error("expected fuzzy sub match on soname with version suffix")
	}
	if hasLib(map[string]bool{"python3": true}, "libonnxruntime.so") {
		t.Error("python3 should not fuzzy-match onnxruntime")
	}
}