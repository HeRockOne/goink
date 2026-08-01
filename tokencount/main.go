package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"novel/internal/agentcfg"
	"novel/internal/llm"
	"novel/internal/mcp_tools"
	"novel/internal/skill"
)

func main() {
	total := 0

	// 1. Identity
	identity := agentcfg.AgentIdentity(agentcfg.MainAgent)
	n, _ := llm.CountTokens(identity)
	fmt.Printf("1. Identity (mainAgentSystem1):    %6d tokens\n", n)
	total += n

	// 2a. 扫描 skills/ 目录获取 always 技能
	var always []*skill.Skill
	entries, _ := os.ReadDir("skills")
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		sk, err := skill.ParseFile("skills/" + e.Name())
		if err != nil {
			continue
		}
		if sk.Mode == skill.ModeAlways {
			always = append(always, sk)
		}
	}

	// 2b. 扫描 internal/skill/builtin/ 目录获取 auto 技能（catalog 来源）
	var autoMeta []skill.SkillMeta
	entries, _ = os.ReadDir("internal/skill/builtin")
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		sk, err := skill.ParseFile("internal/skill/builtin/" + e.Name())
		if err != nil {
			continue
		}
		if sk.Mode == skill.ModeAuto {
			autoMeta = append(autoMeta, sk.Meta("builtin"))
		}
	}

	// 3. Always skills（全量正文注入）
	alwaysTokens := 0
	for _, sk := range always {
		nt, _ := llm.CountTokens(sk.Content)
		fmt.Printf("3. Always %-28s %6d tokens\n", sk.Name+":", nt)
		alwaysTokens += nt
	}
	total += alwaysTokens

	// 4. Skill catalog（auto 模式，仅 name + description）
	catalog := agentcfg.BuildSkillCatalog(autoMeta)
	nt, _ := llm.CountTokens(catalog)
	fmt.Printf("4. Skill catalog (%d auto skills):  %6d tokens\n", len(autoMeta), nt)
	total += nt

	// 5. Tool definitions
	registry := mcp_tools.NewRegistry(nil)
	mcp_tools.RegisterAllTools(registry)
	tools := registry.OpenAI(nil)
	toolsJSON, _ := json.Marshal(tools)
	nt, _ = llm.CountTokens(string(toolsJSON))
	fmt.Printf("5. Tool definitions (%d tools):     %6d tokens\n", len(tools), nt)
	total += nt

	fmt.Printf("\n=== Total initial injection: %d tokens ===\n", total)
}
