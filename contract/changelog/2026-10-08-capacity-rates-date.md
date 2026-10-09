# The capacity plan's exchange rates carry their date

`CapacityPrices` gains `rates_date` (the European Central Bank's date of the oldest rate the plan
uses, `YYYY-MM-DD`, empty with none) and `rates_fetched_at` (when the oldest of them was fetched,
null with none). `nzd_per` holds only the rates the plan uses: the server price's currency unless
it is NZD, and USD. The rates now come from the bank's daily reference rates rather than a setting.
