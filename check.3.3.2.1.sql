use tpcc;
SELECT 1/(count(*)-1)
FROM warehouse
FULL OUTER JOIN
(SELECT d_w_id, sum(d_ytd) as sum_d_ytd FROM district GROUP BY d_w_id)
ON (w_id = d_w_id)
WHERE w_ytd != sum_d_ytd;
