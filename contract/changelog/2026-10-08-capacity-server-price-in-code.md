# The capacity plan always has a server price

`CapacityPrices.currency` and `server_cents` are always set: the price of the server Whisk runs,
kept in the control plane's code, rather than an optional setting. `set` now says only whether
the costs can be shown in NZ dollars, which needs an exchange rate for that currency.
