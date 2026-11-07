# finlang Language Support

`.fin` 파일의 문법을 강조하고 현재 위치에서 사용할 수 있는 변수, 함수, 내장 함수와 키워드를 제안하는 VS Code 확장입니다. **Ctrl+Space**로 후보를 다시 열고, 함수를 선택한 뒤 **Tab**으로 인수 자리를 이동할 수 있습니다.

| 입력 | 후보 | 하는 일 |
| --- | --- | --- |
| `set` | `settle` | Money를 통화 그대로 정산합니다. |
| `rou` | `round` | Decimal을 지정한 자리로 반올림합니다. |
| `input annual_rate as dec` | `decimal` | Decimal 입력 선언을 완성합니다. |

## 선언한 변수와 함수

```fin
let annual_rate = 0.05;
let calculate = fn(principal) {
    return principal * annual_rate;
};
print(calculate(1000000));
```

함수 본문에서 `pri`와 `annu`를 입력하면 매개변수 `principal`과 바깥 변수 `annual_rate`를 제안합니다. 마지막 줄에서 `calc`를 입력하면 `calculate(principal)` 호출을 삽입합니다. 조건문 안의 선언은 그 분기에서만 후보에 표시됩니다. `rule`의 각 단계에서는 입력값과 앞선 단계에서 선언한 값도 사용할 수 있습니다.

후보는 현재 파일의 커서 앞 선언을 기준으로 합니다. 자기 자신을 호출하는 함수는 본문에서도 이름이 표시됩니다. 파일 전체의 실행 순서나 동적 함수 값은 분석 범위 밖입니다. `.fin`에서는 VS Code의 일반 단어 추천을 기본으로 끕니다.

## 설치

VS Code의 **확장** 패널에서 **… → VSIX에서 설치**를 선택해 `finlang-language-0.2.0.vsix` 파일을 엽니다. 터미널에서는 `code --install-extension 경로\finlang-language-0.2.0.vsix`를 실행할 수 있습니다.

코드 실행은 체험판의 `fin.exe` 또는 이미 등록된 VS Code 작업을 사용합니다.

## 소스에서 패키징

`editors/vscode`에서 `npm test`를 실행해 후보와 인수 입력 동작을 확인합니다. VSIX는 `npx @vscode/vsce package --out ../../build/finlang-language-0.2.0.vsix`로 만듭니다.
