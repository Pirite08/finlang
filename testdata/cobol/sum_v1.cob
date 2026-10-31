identification division.
program-id. sum-v1.
environment division.
configuration section.
repository.
    function all intrinsic.
data division.
working-storage section.
*> Sum first, then settle once: no input or intermediate rounding.
01 amount-a           pic s9(9)v9(3) usage packed-decimal.
01 amount-b           pic s9(9)v9(3) usage packed-decimal.
01 amount-c           pic s9(9)v9(3) usage packed-decimal.
01 raw-sum            pic s9(15)v9(14) usage packed-decimal.
01 sum-receiver       pic s9(9)v99 usage packed-decimal.
01 size-failed        pic 9.
01 input-line         pic x(2048).
01 case-count-text    pic x(32).
01 case-count         binary-long unsigned.
01 case-index         binary-long unsigned.
01 case-id            pic x(128).
01 amount-a-text      pic x(256).
01 amount-b-text      pic x(256).
01 amount-c-text      pic x(256).
01 output-amount-text pic z(8)9.99.
procedure division.
    accept case-count-text
    compute case-count = function numval(case-count-text)
    perform varying case-index from 1 by 1
        until case-index > case-count
        accept input-line
        move spaces to case-id amount-a-text amount-b-text amount-c-text
        unstring input-line delimited by "|"
            into case-id amount-a-text amount-b-text amount-c-text
        end-unstring
        move 0 to size-failed
        compute amount-a = function numval(amount-a-text)
            on size error move 1 to size-failed
        end-compute
        compute amount-b = function numval(amount-b-text)
            on size error move 1 to size-failed
        end-compute
        compute amount-c = function numval(amount-c-text)
            on size error move 1 to size-failed
        end-compute
        if size-failed = 0
            compute raw-sum = amount-a + amount-b + amount-c
                on size error move 1 to size-failed
            end-compute
        end-if
        if size-failed = 0
            compute sum-receiver
                rounded mode is nearest-even = raw-sum
                on size error move 1 to size-failed
            end-compute
        end-if
        if size-failed = 1
            display function trim(case-id)
                "|error|E_LEGACY_RANGE|none"
        else
            move function abs(sum-receiver) to output-amount-text
            if sum-receiver < 0
                display function trim(case-id) "|ok|-"
                    function trim(output-amount-text) "|none"
            else
                display function trim(case-id) "|ok|"
                    function trim(output-amount-text) "|none"
            end-if
        end-if
    end-perform
    goback.
