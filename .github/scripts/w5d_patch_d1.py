from pathlib import Path
import re


def must_replace(text: str, old: str, new: str, label: str) -> str:
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{label}: expected exactly 1 match, got {count}")
    return text.replace(old, new, 1)


def remove_budget_refuse_blocks(text: str) -> tuple[str, int]:
    needle = "if err := h.budget.Refuse(); err != nil {"
    removed = 0
    while needle in text:
        start = text.index(needle)
        # Include indentation at the start of the line.
        line_start = text.rfind("\n", 0, start) + 1
        depth = 0
        i = start
        seen_open = False
        while i < len(text):
            ch = text[i]
            if ch == "{":
                depth += 1
                seen_open = True
            elif ch == "}":
                depth -= 1
                if seen_open and depth == 0:
                    end = i + 1
                    # Consume one trailing newline if present.
                    if end < len(text) and text[end] == "\n":
                        end += 1
                    text = text[:line_start] + text[end:]
                    removed += 1
                    break
            i += 1
        else:
            raise SystemExit("unterminated h.budget.Refuse block")
    return text, removed


host_path = Path("internal/host/host.go")
host = host_path.read_text(encoding="utf-8")

host = must_replace(
    host,
    '\tbudget          *BudgetSentinel     // 预算政策；未启用为 nil（方法 nil 安全）\n',
    '',
    'host budget field',
)

start_marker = '\t// 预算哨兵:Engine 在每轮循环边界直接调用 HandleBoundary(不再经事件订阅)。\n'
end_marker = '\t// 统一前进闸门：执行一次性 hold，并阻止 review 模式下无许可的新章。\n'
if host.count(start_marker) != 1 or host.count(end_marker) != 1:
    raise SystemExit("host sentinel marker mismatch")
start = host.index(start_marker)
end = host.index(end_marker, start)
host = host[:start] + host[end:]

host = host.replace('\t\tbudget:    h.budget,\n', '')
if 'budget:    h.budget' in host:
    raise SystemExit('host engine budget wiring still present')

host, removed = remove_budget_refuse_blocks(host)
if removed < 5:
    raise SystemExit(f"expected at least 5 budget refuse blocks, removed {removed}")

host = host.replace('\t\tBudgetLimitUSD:         h.budget.Limit(),\n', '\t\tBudgetLimitUSD:         0,\n')
if 'h.budget.' in host or 'h.budget)' in host or 'h.budget,' in host:
    raise SystemExit('host still contains h.budget reference')

host_path.write_text(host, encoding="utf-8")

engine_path = Path("internal/host/engine.go")
engine = engine_path.read_text(encoding="utf-8")
engine = must_replace(
    engine,
    '\tbudget    *BudgetSentinel\n',
    '',
    'engine budget field',
)

pattern = re.compile(
    r'\n\t\t// 政策边界:预算止损优先于验收/推进暂停。\n'
    r'\t\tif e\.budget\.HandleBoundary\(\) \{\n'
    r'\t\t\treturn\n'
    r'\t\t\}\n'
)
engine, n = pattern.subn('\n', engine)
if n != 1:
    raise SystemExit(f"engine budget boundary: expected 1 match, got {n}")
if 'e.budget' in engine or 'BudgetSentinel' in engine:
    raise SystemExit('engine still contains budget runtime reference')
engine_path.write_text(engine, encoding="utf-8")

print(f"W5D D1 patch applied; removed {removed} Host budget refusal blocks")
