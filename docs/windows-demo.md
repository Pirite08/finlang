# finlang Windows 체험판

Windows x64용 ZIP을 쓰기 가능한 폴더에 모두 풀고 `run-demo.cmd`를 두 번 클릭하세요. Go나 API 키 없이 단리 계산과 감사 기록 재현을 차례대로 실행합니다. 원금 100만 원에 연이율 5%를 반년 적용한 결과는 25,000원입니다. 계산 결과와 재현 결과는 모두 `expected.json`과 같은 내용을 담습니다.

```json
{"schema_version":"1","rule":"interest_v1","status":"ok","result":{"currency":"KRW","amount":"25000"}}
```

| 파일 | 역할 |
| --- | --- |
| `fin.exe` | 규칙을 실행하는 인터프리터 |
| `run-demo.cmd` | 단리 계산과 감사 재현을 연속 실행 |
| `interest_rule_v1.fin` | 이자 계산 단계와 원 단위 정산 방식 |
| `interest.json` | 원금, 연이율, 기간 |
| `interest_rule_v1.contract.json` | 입출력의 자료형, 통화, 허용 범위 |
| `expected.json` | 기본 입력에 대한 예상 결과 |
| `Go-LICENSE.txt` | 실행 파일에 포함된 Go 코드의 라이선스 고지 |

## PowerShell에서 직접 실행하기

압축을 푼 폴더에서 PowerShell을 열고 다음 명령을 실행하세요. `$audit`에 이번 실행의 감사 기록 파일 이름을 저장합니다.

```powershell
$audit = "interest-$([guid]::NewGuid().ToString('N')).audit.jsonl"
.\fin.exe -file interest_rule_v1.fin -contract interest_rule_v1.contract.json -input interest.json -json -rule-only -audit $audit
.\fin.exe -file interest_rule_v1.fin -contract interest_rule_v1.contract.json -json -replay-audit $audit
```

감사 재현은 감사 기록에 저장된 입력을 사용합니다. 기존 기록을 재현하려면 계산에 쓴 규칙과 계약, 같은 버전의 `fin.exe`를 보관하세요. 감사 기록에는 실제 입력과 결과가 들어가므로 샘플을 바꿔 볼 때는 테스트 데이터를 사용하세요.

`interest.json`에서 `annual_rate`의 값을 `"0.06"`으로 바꾸고 다시 실행하면 30,000원이 나옵니다. `expected.json`은 기본 입력의 기준 결과로 그대로 남겨 두세요.
