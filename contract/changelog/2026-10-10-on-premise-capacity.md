# Capacity's money is optional

In the wire types, `Capacity.prices` and `Capacity.revenue` are optional, and each
`CapacityRow`'s `monthly_cents`, `per_gb_cents`, `revenue_cents` and `share_percent` is left out
when the plan does not have it rather than null. Whisk On-Premise answers no prices or revenue.
