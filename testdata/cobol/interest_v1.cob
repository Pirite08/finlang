identification division.
program-id. interest-v1.
environment division.
configuration section.
repository.
    function all intrinsic.
data division.
working-storage section.
*> The host validates the contract before sending a decimal string.
*> Business input fields and both arithmetic receivers are packed decimal.
01 principal          pic s9(9)v99 usage packed-decimal.
01 annual-rate        pic 9v9(6) usage packed-decimal.
01 years              pic 99v9(6) usage packed-decimal.
01 raw-interest       pic s9(15)v9(14) usage packed-decimal.
01 interest-receiver  pic s9(9) usage packed-decimal.
01 size-failed        pic 9.
01 input-line         pic x(2048).
01 case-count-text    pic x(32).
01 case-count         binary-long unsigned.
01 case-index         binary-long unsigned.
01 case-id            pic x(128).
01 principal-text     pic x(256).
01 annual-rate-text   pic x(256).
01 years-text         pic x(256).
01 amount-text        pic z(8)9.
procedure division.
    accept case-count-text
    compute case-count = function numval(case-count-text)
    perform varying case-index from 1 by 1
        until case-index > case-count
        accept input-line
        move spaces to case-id principal-text annual-rate-text years-text
        unstring input-line delimited by "|"
            into case-id principal-text annual-rate-text years-text
        end-unstring
        move 0 to size-failed
        compute principal = function numval(principal-text)
            on size error move 1 to size-failed
        end-compute
        compute annual-rate = function numval(annual-rate-text)
            on size error move 1 to size-failed
        end-compute
        compute years = function numval(years-text)
            on size error move 1 to size-failed
        end-compute
        if size-failed = 0
            compute raw-interest = principal * annual-rate * years
                on size error move 1 to size-failed
            end-compute
        end-if
        if size-failed = 0
            compute interest-receiver
                rounded mode is nearest-away-from-zero = raw-interest
                on size error move 1 to size-failed
            end-compute
        end-if
        if size-failed = 1
            display function trim(case-id)
                "|error|E_LEGACY_RANGE|none"
        else
            move interest-receiver to amount-text
            display function trim(case-id) "|ok|"
                function trim(amount-text) "|none"
        end-if
    end-perform
    goback.
