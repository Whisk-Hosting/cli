# Errors: RELEASE_NOT_COVERED

A new code for a Whisk On-Premise licence asking whisk.run for a release bundle it does not cover:
one published after the licence's last day, or any before its first day (403). Only whisk.run
answers it. `RATE_LIMITED` gains the scope `onpremise_releases`, and `LICENCE_INVALID` is also
whisk.run's answer to a licence file sent to the release downloads that does not verify.
