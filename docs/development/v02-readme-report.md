# v0.2 README 작업 기록

- README를 소개, 빠른 실행, 정산 시점, JSON 계약, trace, 한도, 구조, 검증, 로드맵 순서로 재작성했다.
- Money·Decimal·계약·trace 구현, 내장 함수 테스트, 예제 3종과 계약·입력 fixture, v0.1 검증 문서를 근거로 작성했다. USD 1.00/1.02 사례는 기존 TestMoneySettlementTiming과 일치한다.
- 끝자리 0과 숫자 값·표시 형식 구분, 통화 오류, EXACT 손실 거부, JSON 숫자 문자열, 암묵적 출력 정산 금지, trace 절단을 설명했다.
- v0.1 생성 사례 수는 과거 기준임을 명시했다. 최종 v0.2 테스트 수, 성능 우위, 운영 적합성, COBOL 호환성, 소유자·라이선스·CI 통과 배지는 주장하지 않았다.

## 통합 시 확인할 사항

- 루트 담당자가 통합 중인 CLI의 -file/-contract/-input/-json/-trace 조합과 fin v0.2.0 버전을 사용한다. CLI 완료 후 README 이자 명령과 3개 fixture 결과를 실행 확인해야 한다.
- docs/evaluation/v0.2-verification.md 링크는 루트 담당자가 작성할 최종 검증 문서를 대상으로 한다.
- GitHub 소유자 결정 후 필요한 경우 clone URL을 추가한다. 임의의 원격 주소는 넣지 않았다.

## 실행 확인

- go test ./internal/eval -run 'TestMoney(SettlementTiming|Expressions|Errors)$' -count=1: 통과.
- go test ./internal/contract ./internal/trace -count=1: 통과.
- git diff --check -- README.md docs/development/v02-readme-report.md: 공백 오류 없음. Windows CRLF 정규화 경고만 출력.
