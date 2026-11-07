package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const benchmarkPassphrase = "rizzbase-benchmark-only"

var benchmarkYapCounts = []int{100, 1000, 5000, 10000, 20000}

func benchmarkProject(b *testing.B, yapCount int) (string, string, string) {
	b.Helper()
	path := filepath.Join(b.TempDir(), "benchmark.rizz")
	_, err := CreateProject(path, "Benchmark", benchmarkPassphrase)
	if err != nil {
		b.Fatal(err)
	}
	lobby, err := CreateLobby(path, benchmarkPassphrase, "Benchmark lobby")
	if err != nil {
		b.Fatal(err)
	}
	for start := 0; start < yapCount; start += MaxBulkYaps {
		end := start + MaxBulkYaps
		if end > yapCount {
			end = yapCount
		}
		batch := make([]YapInput, 0, end-start)
		for i := start; i < end; i++ {
			batch = append(batch, YapInput{
				ID: fmt.Sprintf("yap-%08d", i),
				Data: json.RawMessage(fmt.Sprintf(
					`{"group":"group-%02d","score":%d,"payload":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}`,
					i%20, i,
				)),
			})
		}
		if _, err := CreateYaps(path, benchmarkPassphrase, lobby.ID, batch); err != nil {
			b.Fatal(err)
		}
	}
	return path, lobby.ID, fmt.Sprintf("yap-%08d", 0)
}

func benchmarkBySize(b *testing.B, run func(*testing.B, int, string, string, string)) {
	b.Helper()
	for _, yapCount := range benchmarkYapCounts {
		b.Run(fmt.Sprintf("Yaps=%d", yapCount), func(b *testing.B) {
			b.StopTimer()
			path, lobbyID, yapID := benchmarkProject(b, yapCount)
			info, err := os.Stat(path)
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(info.Size())
			b.ReportMetric(float64(info.Size()), "project_bytes")
			b.ReportAllocs()
			b.StartTimer()
			run(b, yapCount, path, lobbyID, yapID)
		})
	}
}

func BenchmarkOpenProject(b *testing.B) {
	benchmarkBySize(b, func(b *testing.B, _ int, path, _, _ string) {
		for i := 0; i < b.N; i++ {
			if _, err := OpenProject(path, benchmarkPassphrase); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkQueryYapPage(b *testing.B) {
	benchmarkBySize(b, func(b *testing.B, yapCount int, path, lobbyID, _ string) {
		limit := 100
		if limit > yapCount {
			limit = yapCount
		}
		options := QueryOptions{
			Where:   []Filter{{Field: "score", Operator: ">=", Value: json.RawMessage("0")}},
			OrderBy: &OrderBy{Field: "score", Direction: "desc"},
			Limit:   &limit,
		}
		for i := 0; i < b.N; i++ {
			if _, err := QueryYapPage(path, benchmarkPassphrase, lobbyID, options); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkUpdateYap(b *testing.B) {
	benchmarkBySize(b, func(b *testing.B, _ int, path, lobbyID, yapID string) {
		for i := 0; i < b.N; i++ {
			if err := UpdateYap(path, benchmarkPassphrase, lobbyID, yapID, map[string]any{
				"group":   "group-00",
				"score":   i,
				"payload": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			}, time.Second); err != nil {
				b.Fatal(err)
			}
		}
	})
}
