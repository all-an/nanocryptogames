# Access Log Queries

## Last 50 accesses
```sql
SELECT accessed_at, ip, country, path FROM access_log ORDER BY accessed_at DESC LIMIT 50;
```

## Daily totals
```sql
SELECT day, count FROM access_daily ORDER BY day DESC;
```

## Top countries
```sql
SELECT country, count(*) FROM access_log GROUP BY country ORDER BY count DESC;
```

## Top paths
```sql
SELECT path, count(*) FROM access_log GROUP BY path ORDER BY count DESC;
```

## Accesses today
```sql
SELECT count FROM access_daily WHERE day = CURRENT_DATE;
```
