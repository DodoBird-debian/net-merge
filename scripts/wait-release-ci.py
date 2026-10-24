#!/usr/bin/env python3
"""Publish only after the existing full CI succeeds on the exact source commit."""
import json
import os
import subprocess
import time
repo, sha = os.environ['GITHUB_REPOSITORY'], os.environ['GITHUB_SHA']
endpoint = f'repos/{repo}/actions/workflows/ci.yml/runs?head_sha={sha}&event=push'
for _ in range(60):
    data = json.loads(subprocess.check_output(['gh','api',endpoint]))
    runs = data.get('workflow_runs',[])
    if runs:
        run = runs[0]
        if run['status'] == 'completed':
            if run['conclusion'] != 'success':
                raise SystemExit(f"Full CI did not pass: {run['html_url']} ({run['conclusion']})")
            print('Full CI passed:',run['html_url']); break
    print('Waiting for full CI on',sha,flush=True)
    time.sleep(15)
else:
    raise SystemExit('Full CI has not passed; release remains unpublished. Rerun after CI completes.')
