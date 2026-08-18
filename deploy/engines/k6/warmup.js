// Build-time smoke script (see the Dockerfile): a minimal k6 script the
// warm-up run drives through bzt's k6 executor. The target is deliberately
// dead -- 127.0.0.1:1 -- because the point is proving the toolchain chain
// (bzt -> k6 -> reporter), not generating load.
import http from 'k6/http';

export default function () {
  http.get('http://127.0.0.1:1/');
}
