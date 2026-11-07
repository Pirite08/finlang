@echo off
setlocal EnableExtensions DisableDelayedExpansion
set "RANDOM="
set "ERRORLEVEL="
set "status=1"
pushd "%~dp0"
if errorlevel 1 goto :exit

:newAudit
set "audit=interest-%RANDOM%-%RANDOM%-%RANDOM%.audit.jsonl"
if exist "%audit%" goto :newAudit

echo [1/2] Calculate interest
.\fin.exe -file interest_rule_v1.fin -contract interest_rule_v1.contract.json -input interest.json -json -rule-only -audit "%audit%"
set "status=%ERRORLEVEL%"
if not "%status%"=="0" goto :finish

echo.
echo [2/2] Replay the audit
.\fin.exe -file interest_rule_v1.fin -contract interest_rule_v1.contract.json -json -replay-audit "%audit%"
set "status=%ERRORLEVEL%"
if "%status%"=="0" echo Audit file: %audit%

:finish
popd

:exit
if not "%~1"=="--no-pause" pause
exit /b %status%
