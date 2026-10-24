import io
import json
import sys
import unittest
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from r27_github_transport import github_getter, _NoRedirect
from r27_github_origin import OriginError

PATH="repos/mcpdev80/baseharbor/actions/runs/123"
class Response:
 def __init__(self, data=b'{"id":123}', status=200):
  self.data=io.BytesIO(data);self.status=status
 def __enter__(self):return self
 def __exit__(self,*args):return False
 def getcode(self):return self.status
 def read(self,n):return self.data.read(n)

class TransportTests(unittest.TestCase):
 def test_authenticated_exact_api(self):
  captured=[]
  def opener(request,timeout):
   captured.append((request.full_url,request.get_header("Authorization"),timeout))
   return Response()
  self.assertEqual(github_getter("redacted-token",opener=opener)(PATH)["id"],123)
  self.assertEqual(captured[0],("https://api.github.com/"+PATH,"Bearer redacted-token",15))
 def test_forbidden_paths_fail_before_network(self):
  def no_network(*a,**k):raise AssertionError("should not call network")
  getter=github_getter("token",opener=no_network)
  for path in ("https://evil.invalid", "../users", PATH+"/logs",PATH+"?x=1",PATH+"/attempts/1/jobs?per_page=99&page=1", PATH+"/attempts/1/jobs?per_page=100&page=0"):
   with self.subTest(path=path),self.assertRaises(OriginError):getter(path)
 def test_bounded_and_valid_json(self):
  for response in (Response(b'x'),Response(b'x'*(4*1024*1024+1)),Response(status=403)):
   with self.assertRaises(OriginError):
    github_getter("token",opener=lambda request,timeout,response=response:response)(PATH)
 def test_no_credential_leak(self):
  for token in ("", "foo\nbar",None):
   with self.assertRaises(OriginError):github_getter(token)
  def denied(request,timeout):raise RuntimeError("do not show token")
  with self.assertRaisesRegex(OriginError,"transport failed") as failure:
   github_getter("secret-do-not-leak",opener=denied)(PATH)
  self.assertNotIn("secret-do-not-leak",str(failure.exception))


class TransportSecurityRegressions(unittest.TestCase):
 def test_cross_origin_redirect_is_denied(self):
  handler=_NoRedirect()
  with self.assertRaisesRegex(OriginError,"redirect denied"):
   handler.redirect_request(None,None,302,"Found",{},"https://evil.invalid/secret")
 def test_ambiguous_api_json_is_denied(self):
  for payload in (b'{"id":1,"id":2}',b'{"id":NaN}',b'{"id":Infinity}'):
   with self.subTest(payload=payload),self.assertRaises(OriginError):
    github_getter("token",opener=lambda req,timeout:Response(payload))(PATH)
