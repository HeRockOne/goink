import tiktoken
import glob, re, json

enc = tiktoken.get_encoding('o200k_base')
total = 0

# 1. Identity
identity = open('C:/Users/Sophia/Desktop/wokspace/no-dy-panel/goink-fork/internal/agentcfg/identity.go', 'r', encoding='utf-8').read()
start = identity.find('const mainAgentSystem1 = `') + len('const mainAgentSystem1 = `')
end = identity.find('`', start)
id_text = identity[start:end]
id_tokens = len(enc.encode(id_text))
print(f'1. Identity (mainAgentSystem1):    {id_tokens:>6} tokens')
total += id_tokens

# 2. Always skills
for name in ['writing-kernel', 'ai-communication-standard']:
    try:
        data = open(f'C:/Users/Sophia/Desktop/wokspace/no-dy-panel/goink-fork/skills/{name}.md', 'r', encoding='utf-8').read()
        t = len(enc.encode(data))
        print(f'2. Skill {name}.md:{" "*(28-len(name))} {t:>6} tokens')
        total += t
    except Exception as e:
        print(f'  skip {name}: {e}')

# 3. NovelState (goink.md)
goink = open('D:/Goink/novels/11/goink.md', 'r', encoding='utf-8').read()
gt = len(enc.encode(goink))
print(f'3. NovelState (goink.md):          {gt:>6} tokens')
total += gt

# 4. Tool definitions - count from tool schema JSON
# Read all tool files and extract descriptions
tool_dir = 'C:/Users/Sophia/Desktop/wokspace/no-dy-panel/goink-fork/internal/mcp_tools'
tool_text_parts = []
for f in glob.glob(f'{tool_dir}/*_tools.go'):
    content = open(f, 'r', encoding='utf-8').read()
    # Extract Description() return strings
    descs = re.findall(r'Description\(\)\s+string\s+\{\s+return\s+`([^`]+)`', content)
    tool_text_parts.extend(descs)
    # Also extract from string literals
    descs2 = re.findall(r'Description\(\)\s+string\s+\{\s+return\s+"([^"]+)"', content)
    tool_text_parts.extend(descs2)

tool_text = '\n'.join(tool_text_parts)
tool_tokens = len(enc.encode(tool_text))
print(f'4. Tool descriptions ({len(tool_text_parts)} items):  {tool_tokens:>6} tokens')
total += tool_tokens

# Also count tool parameter schemas (approximate)
# Each tool has ~200-500 chars of JSON Schema for parameters
param_text = ''
for f in glob.glob(f'{tool_dir}/*_tools.go'):
    content = open(f, 'r', encoding='utf-8').read()
    # Extract jsonschema tags
    tags = re.findall(r'jsonschema:"([^"]+)"', content)
    param_text += ' '.join(tags)
param_tokens = len(enc.encode(param_text))
print(f'5. Tool param schemas (tags):      {param_tokens:>6} tokens')
total += param_tokens

print(f'\n=== Total initial injection: {total} tokens ===')
