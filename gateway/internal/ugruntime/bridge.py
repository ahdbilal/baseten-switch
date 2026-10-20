"""Read-only adapter to a pinned official ug installation. JSON in / JSON out."""
import importlib.metadata
import json
import os
import sys
from urllib.parse import urlsplit

revision = sys.argv[1]
distribution = importlib.metadata.distribution("unity-gateway")
origin = json.loads(distribution.read_text("direct_url.json") or "{}")
if origin.get("vcs_info", {}).get("commit_id") != revision:
    raise RuntimeError("The installed ug revision does not match the tested adapter")

# Compass owns routing; inherited upstream smart-routing flags must not add a
# second router. These changes exist only in this short-lived subprocess.
from ucode.constants import SMART_ROUTING_ENV_KEYS
for key in SMART_ROUTING_ENV_KEYS:
    os.environ[key] = "0"
os.environ.pop("SMART_ROUTER_CONFIG_VERSION", None)

request = json.load(sys.stdin)
client = request["client"]
result = {"revision": revision, "config": {}}
if client == "check":
    from ucode.agents import claude, codex, pi
    from ucode.agents.desktop.claude_config import render_profile
    assert all(callable(f) for f in (claude.render_overlay, codex.render_overlay, pi.render_overlay, render_profile))
else:
    from ucode.databricks import build_auth_token_argv
    workspace, profile = request["workspace"], request["profile"]
    target = urlsplit(request["base_url"])
    origin_url = urlsplit(workspace)
    if origin_url.scheme != "https" or not origin_url.hostname or origin_url.username:
        raise ValueError("Invalid workspace origin")
    if target.scheme != "http" or target.hostname != "127.0.0.1" or target.username or target.query or target.fragment:
        raise ValueError("The app overlay must use its local loopback proxy")
    base = request["base_url"]
    model = request.get("model") or None
    if client in ("codex", "codex-desktop"):
        import tomlkit
        from ucode.agents.codex import render_overlay
        overlay = render_overlay(workspace, model, databricks_profile=profile)
        provider = overlay["model_providers"][overlay["model_provider"]]
        provider.update(base_url=base, requires_openai_auth=False, supports_websockets=False)
        provider["http_headers"]["X-Unity-Gateway-Codex-Picker"] = "mixed-v1"
        # Keep headers inline so the existing model-picker updates can preserve
        # ug attribution while editing the app's single routing marker.
        headers = tomlkit.inline_table()
        headers.update(provider["http_headers"])
        provider["http_headers"] = headers
        name = request["provider"]
        result["config"] = {"model_provider": name, "model": model, "model_providers": {name: provider}}
        result["provider_toml"] = tomlkit.dumps({"model_providers": {name: provider}})
    elif client == "claude-code":
        from ucode.agents.claude import render_overlay
        overlay, _ = render_overlay(workspace, model, profile=profile)
        overlay["env"]["ANTHROPIC_BASE_URL"] = base
        overlay["env"]["CLAUDE_CODE_ATTRIBUTION_HEADER"] = "1"
        result["config"] = overlay
    elif client == "pi":
        from ucode.agents.pi import build_pi_api_key, render_overlay
        ids = [m["id"] for m in request["models"]]
        overlay, _ = render_overlay(model or ids[0], build_pi_api_key({"workspace": workspace, "profile": profile}), {"openai": base}, {}, ids, [])
        provider = overlay["providers"]["databricks-openai"]
        provider["headers"]["X-Unity-Gateway-Codex-Picker"] = "mixed-v1"
        result["config"] = provider
    elif client == "claude-desktop":
        from ucode.agents.desktop.claude_config import render_profile
        argv = build_auth_token_argv(workspace, profile)
        result["config"] = render_profile(
            model_ids=[m["id"] for m in request["models"]], base_url=base,
            helper_command=argv[0], helper_args=argv[1:] + ["--browser-login"],
            model_id_to_display_name={m["id"]: m["name"] for m in request["models"]},
        )
    else:
        raise ValueError("Unsupported client")
json.dump(result, sys.stdout)
