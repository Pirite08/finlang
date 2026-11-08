*> Each fixture starts from its own balances and emits postings on success.
*> All amounts on the wire are in minor units (KRW won or USD cents).
identification division.
program-id. transfer-v1.
environment division.
configuration section.
repository.
    function all intrinsic.
data division.
working-storage section.
01 max-balance         pic 9(19) usage packed-decimal
                       value 9223372036854775807.
01 input-line          pic x(512).
01 case-count-text     pic x(32).
01 case-count          binary-long unsigned.
01 case-index          binary-long unsigned.
01 case-id             pic x(128).
01 currency-code       pic x(3).
01 source-account      pic x(64).
01 target-account      pic x(64).
01 amount-input        pic x(19).
01 source-input        pic x(19).
01 target-input        pic x(19).
01 fee-input           pic x(19).
01 transfer-amount     pic 9(19) usage packed-decimal.
01 source-initial      pic 9(19) usage packed-decimal.
01 target-initial      pic 9(19) usage packed-decimal.
01 fee-initial         pic 9(19) usage packed-decimal.
01 fee-amount          pic 9(19) usage packed-decimal.
*> Uncommitted sums have one extra digit so range checks cannot wrap.
01 debit-total         pic 9(20) usage packed-decimal.
01 target-total        pic 9(20) usage packed-decimal.
01 fee-total           pic 9(20) usage packed-decimal.
01 source-final        pic 9(19) usage packed-decimal.
01 target-final        pic 9(19) usage packed-decimal.
01 fee-final           pic 9(19) usage packed-decimal.
01 result-status       pic x(18).
01 amount-text         pic z(18)9.
01 fee-text            pic z(18)9.
01 debit-text          pic z(18)9.
01 source-text         pic z(18)9.
01 target-text         pic z(18)9.
01 fee-balance-text    pic z(18)9.
procedure division.
    accept case-count-text
    compute case-count = function numval(case-count-text)
    perform varying case-index from 1 by 1
        until case-index > case-count
        perform read-case
        perform apply-transfer
        perform write-result
    end-perform
    goback.

read-case.
    accept input-line
    move spaces to case-id currency-code source-account target-account
        amount-input source-input target-input fee-input
    unstring input-line delimited by "|"
        into case-id currency-code source-account target-account
            amount-input source-input target-input fee-input
    end-unstring
    compute transfer-amount = function numval(amount-input)
    compute source-initial = function numval(source-input)
    compute target-initial = function numval(target-input)
    compute fee-initial = function numval(fee-input).

apply-transfer.
    move source-initial to source-final
    move target-initial to target-final
    move fee-initial to fee-final
    *> Both supported currencies are already expressed in minor units.
    compute fee-amount rounded mode is nearest-even =
        transfer-amount * 0.05
    compute debit-total = transfer-amount + fee-amount
    evaluate true
        when transfer-amount = 0 or debit-total > max-balance
            move "invalid" to result-status
        when debit-total > source-initial
            move "insufficient_funds" to result-status
        when other
            compute target-total = target-initial + transfer-amount
            compute fee-total = fee-initial + fee-amount
            if target-total > max-balance or fee-total > max-balance
                move "invalid" to result-status
            else
                compute source-final = source-initial - debit-total
                move target-total to target-final
                move fee-total to fee-final
                move "ok" to result-status
            end-if
    end-evaluate.

write-result.
    if result-status not = "ok"
        move 0 to fee-amount debit-total
    end-if
    move fee-amount to fee-text
    move debit-total to debit-text
    move source-final to source-text
    move target-final to target-text
    move fee-final to fee-balance-text
    display function trim(case-id) "|" function trim(result-status)
        "|" currency-code "|" function trim(fee-text)
        "|" function trim(debit-text) "|" function trim(source-text)
        "|" function trim(target-text) "|" function trim(fee-balance-text)
    if result-status = "ok"
        move transfer-amount to amount-text
        display function trim(case-id) "|debit|"
            function trim(source-account) "|" currency-code "|-"
            function trim(debit-text)
        display function trim(case-id) "|credit|"
            function trim(target-account) "|" currency-code "|"
            function trim(amount-text)
        display function trim(case-id) "|fee|fee-" currency-code
            "|" currency-code "|" function trim(fee-text)
    end-if.
