"""Keep the native media publication gate explicit and real-tool-required."""
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parent.parent

class NativeMediaCI(unittest.TestCase):
    def test_verify_requires_native_real_media(self):
        workflow = (ROOT / '.github/workflows/build.yml').read_text()
        verify = workflow.split('  verify:', 1)[1].split('  publish:', 1)[0]
        self.assertIn('sudo apt-get install -y ffmpeg', verify)
        self.assertLess(verify.index('sudo apt-get install -y ffmpeg'), verify.index('bash scripts/test-go.sh'))
        self.assertIn('go test -race -count=3 -v ./internal/modules/mediaprobe/...', verify)
        self.assertIn('MEDIA_TOOL_REQUIRE_FFMPEG: "1"', verify)
        self.assertIn('needs: verify', workflow.split('  publish:', 1)[1])
        self.assertIn('go run github.com/kisielk/errcheck@v1.9.0 -ignoretests ./cmd/... ./internal/... ./migrations/... ./tests/...', verify)
        script = (ROOT / 'scripts/test-go.sh').read_text()
        self.assertIn('command -v ffmpeg', script)
        self.assertIn('-race -count=1 -v', script)

    def test_docker_frontend_copies_shared_contracts_before_typecheck(self):
        dockerfile = (ROOT / 'deploy/Dockerfile').read_text()
        frontend = dockerfile.split(' AS frontend', 1)[1].split(' AS backend', 1)[0]
        contracts = 'COPY internal/platform/httpapi/testdata/contracts/ /src/internal/platform/httpapi/testdata/contracts/'
        self.assertIn(contracts, frontend)
        self.assertLess(frontend.index(contracts), frontend.index('RUN npm run build'))
