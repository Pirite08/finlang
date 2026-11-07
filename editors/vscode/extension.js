const { collectBindings } = require('./scope');

const builtins = [
  ['abs', ['value: Decimal | Money'], 'Decimal | Money', '절댓값을 구합니다. Money의 통화를 유지합니다.'],
  ['amount', ['value: Money'], 'Decimal', 'Money에서 금액을 꺼냅니다.'],
  ['ceil', ['value: Decimal'], 'Decimal', '값 이상의 가장 작은 정수값을 반환합니다.'],
  ['clamp', ['value: Decimal | Money', 'lower: Decimal | Money', 'upper: Decimal | Money'], 'Decimal | Money', '값을 하한과 상한 사이로 제한합니다. 자료형을 통일하고 Money라면 통화도 맞춥니다.'],
  ['convert', ['value: Money', 'quote: FXQuote'], 'Money', '환율로 금액을 다른 통화로 바꿉니다. 절차형 rule에서 사용합니다.'],
  ['currency', ['value: Money'], 'String', 'Money에서 통화 코드를 꺼냅니다.'],
  ['decimal', ['text: String'], 'Decimal', '십진 문자열을 정확한 Decimal로 읽습니다.'],
  ['div', ['value: Decimal | Money', 'divisor: Decimal', 'digits: Decimal', 'mode: String'], 'Decimal | Money', '결과 자릿수와 반올림 방식을 지정해 나눕니다.'],
  ['floor', ['value: Decimal'], 'Decimal', '값 이하의 가장 큰 정수값을 반환합니다.'],
  ['format', ['value: Decimal', 'digits: Decimal'], 'String', '정해진 소수 자릿수의 문자열을 만듭니다. 값이 달라지는 반올림이 필요하면 오류가 납니다.'],
  ['format_money', ['value: Money', 'digits: Decimal'], 'String', '통화와 소수 자릿수를 포함한 표시 문자열을 만듭니다. 정산은 settle에서 지정합니다.'],
  ['max', ['value: Decimal | Money'], 'Decimal | Money', '한 개 이상의 값 중 최댓값을 반환합니다. Money는 통화가 같아야 합니다.', true],
  ['min', ['value: Decimal | Money'], 'Decimal | Money', '한 개 이상의 값 중 최솟값을 반환합니다. Money는 통화가 같아야 합니다.', true],
  ['money', ['currency: String', 'amount: Decimal'], 'Money', '정확한 금액에 KRW 또는 USD 통화를 붙입니다.'],
  ['print', ['value: 값'], 'Unit', '값을 터미널에 출력합니다. JSON 결과 모드와 -rule-only에서는 사용할 수 없습니다.'],
  ['rate', ['base: String', 'target: String'], 'FXQuote', '절차형 rule에서 환율을 읽습니다. -rule-only는 사전에 검증된 환율 스냅샷을 사용합니다.'],
  ['round', ['value: Decimal', 'digits: Decimal', 'mode: String'], 'Decimal', '십진수를 정해진 자리로 반올림합니다. 모드는 "HALF_EVEN", "HALF_UP", "TOWARD_ZERO", "EXACT"입니다.'],
  ['settle', ['value: Money', 'digits: Decimal', 'mode: String'], 'Money', '통화를 유지하며 정해진 소수 자리로 정산합니다. 모드는 "HALF_EVEN", "HALF_UP", "TOWARD_ZERO", "EXACT"입니다.'],
  ['sign', ['value: Decimal | Money'], 'Decimal', '음수, 0, 양수에 대해 각각 -1, 0, 1을 반환합니다.'],
  ['sum', ['value: Decimal | Money'], 'Decimal | Money', '한 개 이상의 값을 왼쪽부터 더합니다. Money는 통화가 같아야 합니다.', true],
  ['trunc', ['value: Decimal'], 'Decimal', '0 방향으로 소수부를 잘라냅니다.'],
];

const keywords = [
  ['as', '절차형 규칙에서 input 이름 as decimal로 Decimal 입력을 선언합니다.'],
  ['else', 'if 조건이 거짓일 때 실행할 블록을 적습니다.'],
  ['false', '거짓을 나타내는 Bool 값입니다.'],
  ['fn', '일반 실행에서 함수 값을 만듭니다. -rule-only에서는 사용할 수 없습니다.'],
  ['if', '괄호 안의 Bool 조건에 따라 블록을 실행합니다.'],
  ['in', '규칙의 Money 입출력 통화를 적습니다. 예시 input principal in "KRW"'],
  ['input', '규칙의 입력 이름과 자료형을 선언합니다.'],
  ['let', '계산 결과나 값을 이름에 연결합니다.'],
  ['output', '규칙의 결과 이름과 통화를 선언합니다.'],
  ['require', 'step 안에서 뒤따르는 Bool 식이 참인지 검사합니다.'],
  ['return', '함수의 값을 돌려주거나 규칙의 선언된 결과 이름을 반환합니다.'],
  ['rule', '입력, 출력, 계산 단계가 있는 규칙을 정의합니다.'],
  ['step', '규칙이 적힌 순서대로 실행하는 계산 단계를 정의합니다.'],
  ['true', '참을 나타내는 Bool 값입니다.'],
];

const identifier = /[A-Za-z_][A-Za-z0-9_]*/g;

function isCode(text) {
  let inString = false;
  for (let i = 0; i < text.length; i++) {
    const char = text[i];
    if (inString && char === '\\') {
      i++;
    } else if (char === '"') {
      inString = !inString;
    } else if (!inString && char === '/' && text[i + 1] === '/') {
      return false;
    }
  }
  return !inString;
}

function createCompletionProvider(vscode) {
  function keywordItem(name, documentation, range) {
    const item = new vscode.CompletionItem(name, vscode.CompletionItemKind.Keyword);
    item.documentation = documentation;
    item.insertText = name;
    item.range = range;
    return item;
  }

  function functionItem(name, params, detail, documentation, range, hasParen) {
    const item = new vscode.CompletionItem(name, vscode.CompletionItemKind.Function);
    item.detail = detail;
    item.documentation = documentation;
    item.range = range;
    if (hasParen) {
      item.insertText = name;
      return item;
    }
    const snippet = new vscode.SnippetString();
    snippet.appendText(`${name}(`);
    params.forEach((param, index) => {
      if (index > 0) snippet.appendText(', ');
      snippet.appendPlaceholder(param);
    });
    snippet.appendText(')');
    item.insertText = snippet;
    return item;
  }

  function builtinItem([name, params, result, documentation, variadic], range, hasParen) {
    const signatureParams = variadic ? [...params, '…'] : params;
    const detail = `${name}(${signatureParams.join(', ')}) → ${result}`;
    return functionItem(name, params.map(param => param.split(':')[0]), detail, documentation, range, hasParen);
  }

  function bindingItem(binding, range, hasParen) {
    const { name, parameters, description } = binding;
    if (parameters) {
      return functionItem(name, parameters, `${name}(${parameters.join(', ')})`, description, range, hasParen);
    }
    const item = new vscode.CompletionItem(name, vscode.CompletionItemKind.Variable);
    item.detail = description;
    item.insertText = name;
    item.range = range;
    return item;
  }

  return {
    provideCompletionItems(document, position) {
      const line = document.lineAt(position).text;
      if (!isCode(line.slice(0, position.character))) return [];

      const range = document.getWordRangeAtPosition(position, identifier) ?? new vscode.Range(position, position);
      const prefix = line.slice(range.start.character, position.character);
      const beforeWord = line.slice(0, range.start.character);
      const hasParen = line.slice(range.end.character).trimStart().startsWith('(');
      if (/^\s*input\s+[A-Za-z_][A-Za-z0-9_]*\s+as\s+$/.test(beforeWord)) {
        return 'decimal'.startsWith(prefix) ? [keywordItem('decimal', '통화가 없는 Decimal 입력을 선언합니다.', range)] : [];
      }

      const { bindings, restricted } = collectBindings(document.getText(), document.offsetAt(range.start));
      const local = [...bindings.values()].filter(({ name }) => name.startsWith(prefix))
        .map(binding => bindingItem(binding, range, hasParen));
      if (restricted) return local;
      const matches = ([name]) => name.startsWith(prefix) && !bindings.has(name);
      return [
        ...local,
        ...builtins.filter(matches).map((spec) => builtinItem(spec, range, hasParen)),
        ...keywords.filter(matches).map(([name, doc]) => keywordItem(name, doc, range)),
      ];
    },
  };
}

function activate(context) {
  const vscode = require('vscode');
  context.subscriptions.push(vscode.languages.registerCompletionItemProvider('finlang', createCompletionProvider(vscode)));
}

module.exports = { activate, createCompletionProvider };
