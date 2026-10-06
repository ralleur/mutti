#!/usr/bin/env python3
"""Playback headroom of a synthetic instance, with or without AI load.

Logs in as a synthetic profile of a running instance (fixture.json from
module-package-smoke.py --setup-only), forces a server-side H.264 transcode of
each library clip over HLS and reads it with ffmpeg as fast as the server
delivers. A speed factor >= 1.0 means the server keeps up with real time.
Writes JSON to stdout; no media leaves the machine.

  playback-under-load.py --fixture build/qualification-en-1/instance/fixture.json \
      --ffmpeg Mutti.app/Contents/Resources/ffmpeg/ffmpeg --label ai-load
"""
import argparse
import json
from pathlib import Path
import re
import subprocess
import time
import urllib.parse
import urllib.request

parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
parser.add_argument('--fixture', type=Path, required=True)
parser.add_argument('--ffmpeg', type=Path, required=True)
parser.add_argument('--label', required=True)
parser.add_argument('--profile', default='alpha')
args = parser.parse_args()
fixture = json.loads(args.fixture.read_text())
base = fixture['jellyfin']
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
auth = 'MediaBrowser Client="Playback probe", Device="Synthetic", DeviceId="playback-probe", Version="0.1"'


def call(path, body=None, token=None):
    headers = {'Authorization': auth + (f', Token="{token}"' if token else ''), 'Content-Type': 'application/json'}
    req = urllib.request.Request(base + path, data=json.dumps(body).encode() if body is not None else None, headers=headers)
    with opener.open(req, timeout=60) as r:
        return json.loads(r.read() or b'null')


profile = fixture['profiles'][args.profile]
login = call('/Users/AuthenticateByName', {'Username': profile['name'], 'Pw': profile['password']})
token, user = login['AccessToken'], login['User']['Id']
items = call(f'/Users/{user}/Items?IncludeItemTypes=Movie&Recursive=true', token=token)['Items']
results = []
for item in items:
    query = urllib.parse.urlencode({'api_key': token, 'MediaSourceId': item['Id'], 'VideoCodec': 'h264', 'AudioCodec': 'aac',
                                    'VideoBitrate': 4000000, 'MaxWidth': 1920, 'MaxHeight': 1080, 'TranscodingMaxAudioChannels': 2,
                                    'SegmentContainer': 'ts', 'MinSegments': 1, 'BreakOnNonKeyFrames': 'true',
                                    'PlaySessionId': f'probe-{int(time.time() * 1000)}', 'DeviceId': 'playback-probe'})
    url = f'{base}/Videos/{item["Id"]}/master.m3u8?{query}'
    started = time.time()
    proc = subprocess.run([str(args.ffmpeg), '-hide_banner', '-nostats', '-progress', 'pipe:1', '-i', url, '-f', 'null', '-'],
                          capture_output=True, text=True, timeout=600)
    wall = time.time() - started
    duration = item.get('RunTimeTicks', 0) / 1e7
    speeds = [float(s) for s in re.findall(r'speed=\s*([\d.]+)x', proc.stdout)]
    results.append({'title': item['Name'], 'seconds': round(duration, 1), 'wallSeconds': round(wall, 1),
                     'speed': speeds[-1] if speeds else None, 'realtimeFactor': round(duration / wall, 2) if wall else None, 'ok': proc.returncode == 0,
                     'error': proc.stderr.strip().splitlines()[-1][:200] if proc.returncode and proc.stderr.strip() else None})
print(json.dumps({'label': args.label, 'at': time.strftime('%Y-%m-%dT%H:%M:%S'), 'results': results}, ensure_ascii=False))
