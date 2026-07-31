package main

import (
	"encoding/json"
	"fmt"
	"os"

	"novel/internal/agentcfg"
	"novel/internal/llm"
	"novel/internal/mcp_tools"
)

func main() {
	total := 0

	// 1. Identity
	identity := agentcfg.AgentIdentity(agentcfg.MainAgent)
	n, _ := llm.CountTokens(identity)
	fmt.Printf("1. Identity (mainAgentSystem1):    %6d tokens\n", n)
	total += n

	// 2. Always skills
	for _, name := range []string{"writing-kernel", "ai-communication-standard"} {
		data, err := os.ReadFile("skills/" + name + ".md")
		if err != nil {
			continue
		}
		nt, _ := llm.CountTokens(string(data))
		fmt.Printf("2. Skill %-28s %6d tokens\n", name+".md:", nt)
		total += nt
	}

	// 3. NovelState (read goink.md directly)
	goink, err := os.ReadFile("D:/Goink/novels/11/goink.md")
	if err == nil {
		nt, _ := llm.CountTokens(string(goink))
		fmt.Printf("3. NovelState (goink.md):          %6d tokens\n", nt)
		total += nt
	}

	// 4. Tool definitions
	registry := mcp_tools.NewRegistry(nil)
	mcp_tools.RegisterAllTools(registry)
	tools := registry.OpenAI(nil)
	toolsJSON, _ := json.Marshal(tools)
	nt, _ := llm.CountTokens(string(toolsJSON))
	fmt.Printf("4. Tool definitions (%d tools):     %6d tokens\n", len(tools), nt)
	total += nt

	fmt.Printf("\n=== Total initial injection: %d tokens ===\n", total)
}
