identification division.
program-id. fee-v1.
environment division.
configuration section.
repository.
    function all intrinsic.
data division.
working-storage section.
*> The host validates the contract before sending a decimal string.
01 amount-input       pic s9(9)v99 usage packed-decimal.
01 raw-fee            pic s9(15)v9(14) usage packed-decimal.
01 fee-receiver       pic s9(9) usage packed-decimal.
01 size-failed        pic 9.
01 applied-branch     pic x(4).
01 input-line         pic x(2048).
01 case-count-text    pic x(32).
01 case-count         binary-long unsigned.
01 case-index         binary-long unsigned.
01 case-id            pic x(128).
01 input-amount-text  pic x(256).
01 output-amount-text pic z(8)9.
procedure division.
    accept case-count-text
    compute case-count = function numval(case-count-text)
    perform varying case-index from 1 by 1
        until case-index > case-count
        accept input-line
        move spaces to case-id input-amount-text
        unstring input-line delimited by "|"
            into case-id input-amount-text
        end-unstring
        move 0 to size-failed
        compute amount-input = function numval(input-amount-text)
            on size error move 1 to size-failed
        end-compute
        if amount-input <= 1000000
            move "low" to applied-branch
            if size-failed = 0
                compute raw-fee = amount-input * 0.001
                    on size error move 1 to size-failed
                end-compute
            end-if
        else
            move "high" to applied-branch
            if size-failed = 0
                compute raw-fee =
                    1000 + (amount-input - 1000000) * 0.0005
                    on size error move 1 to size-failed
                end-compute
            end-if
        end-if
        if size-failed = 0
            compute fee-receiver
                rounded mode is nearest-away-from-zero = raw-fee
                on size error move 1 to size-failed
            end-compute
        end-if
        if size-failed = 1
            display function trim(case-id) "|error|E_LEGACY_RANGE|"
                function trim(applied-branch)
        else
            move fee-receiver to output-amount-text
            display function trim(case-id) "|ok|"
                function trim(output-amount-text) "|"
                function trim(applied-branch)
        end-if
    end-perform
    goback.
