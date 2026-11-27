package sites

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/S-rvo/BlueWhiteThreat/apps/scraper/internal/db"
	"github.com/S-rvo/BlueWhiteThreat/pkg/contract"
	"time"
)

func renderedChunks(source string, links []string) ([]string, error) {
	type event struct {
		Source string
		Links  []string
	}
	var chunks []string
	batch := []string{}
	for _, link := range links {
		candidate := append(append([]string{}, batch...), link)
		data, err := json.Marshal(event{source, candidate})
		if err != nil {
			return nil, err
		}
		if len(data) > 64*1024 {
			if len(batch) == 0 {
				return nil, fmt.Errorf("single rendered URL exceeds message limit")
			}
			prev, _ := json.Marshal(event{source, batch})
			chunks = append(chunks, string(prev))
			batch = []string{link}
			single, _ := json.Marshal(event{source, batch})
			if len(single) > 64*1024 {
				return nil, fmt.Errorf("single rendered URL exceeds message limit")
			}
		} else {
			batch = candidate
		}
	}
	if len(batch) > 0 {
		data, _ := json.Marshal(event{source, batch})
		chunks = append(chunks, string(data))
	}
	return chunks, nil
}
func publishRenderedLinks(ctx context.Context, source string, links []string) error {
	chunks, err := renderedChunks(source, links)
	if err != nil || len(chunks) == 0 {
		return err
	}
	client := db.ConnectRedis()
	defer client.Close()
	c, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	args := make([]interface{}, len(chunks))
	for i, v := range chunks {
		args[i] = v
	}
	// Atomic all-or-nothing admission, including in-flight claims. Replaying a
	// completed page is harmless: URL admission and graph MERGE are idempotent.
	return client.Eval(c, `if redis.call('LLEN',KEYS[1])+redis.call('HLEN','crawler:rendered_inflight')+redis.call('ZCARD','crawler:rendered_retry')+#ARGV>512 then return redis.error_reply('rendered handoff full') end
for _,v in ipairs(ARGV) do redis.call('RPUSH',KEYS[1],v) end return #ARGV`, []string{contract.KeyCrawlerRenderedLinks}, args...).Err()
}
