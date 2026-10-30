-- Seed of the behaviour snapshot fixture. Snapshots run at the pinned
-- reference time NOW = Tuesday 2026-10-27 10:00 UTC, in a UTC session, with
-- weeks starting on Monday. TODAY is 2026-10-27, the week runs from Monday
-- 2026-10-26 to Sunday 2026-11-01, the month from 2026-10-01 to 2026-10-31,
-- {TODAY + 3::BUSINESS_DAYS} is Friday 2026-10-30, and
-- {TODAY + 5::BUSINESS_DAYS} is Tuesday 2026-11-03.
--
-- Each person's due date and last_seen sit on one relative boundary, named
-- after the value; BD is BUSINESS_DAYS, d days, and s seconds. Times are UTC.
--
-- id name        nick   age height balance  active plan country due                            last_seen                                created_at
--  1 Ann Lee     Annie   34 1.68   1250.00  true   pro  France  2026-10-27 TODAY               2026-10-27 10:00:00 NOW                  2024-03-15 09:00
--  2 Bob Stone   NULL    41 1.82   0.00     false  free Japan   2026-10-26 START_OF_WEEK       2026-10-27 09:45:00 NOW - 15 min         2025-01-10 14:30
--  3 Cara Diaz   Cee     29 1.6    -35.50   true   team Brazil  2026-11-01 END_OF_WEEK         2026-10-27 08:30:00 NOW - 90 min         2025-06-01 08:00
--  4 Dan O'Brien NULL    52 NULL   980.25   NULL   free Germany 2026-10-01 START_OF_MONTH      2026-10-27 00:00:00 TODAY                2023-11-20 17:45
--  5 Eve Martin  Evie    23 1.75   15       true   pro  France  2026-10-31 END_OF_MONTH        2026-10-26 23:59:59 TODAY - 1 s          2026-02-14 12:00
--  6 Finn Larsen NULL    67 1.91   NULL     false  team Germany 2026-01-01 START_OF_YEAR       2026-10-26 00:00:00 START_OF_WEEK        2022-07-04 10:00
--  7 Gil Moreau  NULL    38 1.7    300.00   NULL   free NULL    2026-12-31 END_OF_YEAR         2026-10-25 23:59:59 START_OF_WEEK - 1 s  2024-08-08 16:20
--  8 Hana Sato   Hana    45 1.58   2200.10  true   team Japan   2026-10-30 TODAY + 3 BD        2026-10-01 00:00:00 START_OF_MONTH       2025-03-03 07:15
--  9 Ivan Petrov NULL    17 1.77   5.05     true   free Germany 2026-11-03 TODAY + 5 BD        2026-09-30 23:59:59 START_OF_MONTH - 1 s 2026-09-01 18:00
-- 10 Jade Wong   JW      30 1.65   42.00    false  pro  Japan   2025-12-31 START_OF_YEAR - 1 d 2026-01-01 00:00:00 START_OF_YEAR        2025-12-31 23:00
-- 11 Kai Becker  NULL    -1 1.8    0.10     true   free Germany 2027-01-01 END_OF_YEAR + 1 d   2025-12-31 23:59:59 START_OF_YEAR - 1 s  2026-10-20 11:00
-- 12 Lena Novak  Len     61 1.72   10000.00 false  pro  Brazil  2026-11-02 END_OF_WEEK + 1 d   NULL                                     2026-10-27 09:30
-- 13 Mo Reyes    NULL  NULL NULL   NULL     NULL   free NULL    NULL                           NULL                                     2026-05-05 12:00
-- 14 Nora Quinn: soft-deleted, so the scope hides her, although her values
--    match TODAY, NOW, and most other filters.
--
-- So, with the scope applied:
--   due = TODAY                                    Ann
--   due >= START_OF_WEEK AND due <= END_OF_WEEK    Ann, Bob, Cara, Eve, Hana
--   due >= START_OF_MONTH AND due <= END_OF_MONTH  Ann, Bob, Dan, Eve, Hana
--   due >= START_OF_YEAR AND due <= END_OF_YEAR    every dated person but Jade and Kai
--   due = {TODAY + 3::BUSINESS_DAYS}               Hana
--   due = {TODAY + 5::BUSINESS_DAYS}               Ivan
--   last_seen > {NOW - 30::MINUTES}                Ann, Bob
--   last_seen > {NOW - 2::HOURS}                   Ann, Bob, Cara
--   last_seen >= TODAY                             Ann, Bob, Cara, Dan
--   last_seen >= START_OF_WEEK                     ... and Eve, Finn
--   last_seen >= START_OF_MONTH                    ... and Gil, Hana
--   last_seen >= START_OF_YEAR                     ... and Ivan, Jade
--   created_at > {NOW - 1::HOURS}                  Lena
--
-- Edge rows: Gil has no country. Mo has every optional field NULL, and an
-- empty roles array, which is not nullable. Dan has every array empty and
-- Eve every nullable array NULL. Kai's age is -1, a data-entry mistake on
-- which the calculated field age_group fails. Dan's name holds a quote.
--
-- Arrays. friends lists friend_ids as person ids; person N's uid is
-- 00000000-0000-4000-8000-0000000000NN. logins are UTC.
--
--  id tags            lucky   ratings   prices       answers          holidays                logins                              friends  roles
--   1 {go,sql}        {7,13}  {4.5,5}   {9.99,19.90} {true,true}      {2026-10-27,2026-12-24} {2026-10-26 08:00,2026-10-27 10:00} {2,3}    {admin}
--   2 {Go}            {7}     {3}       {120.00}     {false}          {2026-11-02}            {2026-10-27 09:45}                  {1}      {editor}
--   3 {rust,go}       {3,5,8} {2.5,4}   {0.50}       {true,false}     {2026-01-01}            {2026-10-20 12:00,2026-10-27 08:30} {1,8}    {editor,viewer}
--   4 {}              {}      {}        {}           {}               {}                      {}                                  {}       {}
--   5 NULL            NULL    NULL      NULL         NULL             NULL                    NULL                                NULL     {viewer}
--   6 {sql}           {42}    {1}       {5.00,5}     {false,false}    {2026-12-25}            {2026-10-26 00:00}                  {7}      {viewer}
--   7 {travel}        {13}    {3.5}     {}           {true}           {2026-10-31}            NULL                                {6}      {}
--   8 {go,python,sql} {1,2,3} {5,5,4.5} {99.95}      {true,true,true} {2026-10-30,2026-11-01} {2026-09-15 09:00,2026-10-01 00:00} {1,3,10} {admin,editor}
--   9 {python}        {17}    NULL      NULL         {false,true}     NULL                    {2026-09-30 23:59:59}               NULL     {viewer}
--  10 {design,go}     {8}     {4}       {12.00}      {true}           {2025-12-31}            {2026-01-01 00:00}                  {8}      {editor}
--  11 {}              NULL    {}        NULL         {}               {}                      NULL                                {}       {viewer}
--  12 {sql,design}    {99}    {2}       {1000.00}    {false}          {2027-01-01}            {}                                  {1,2}    {admin}
--  13 NULL            NULL    NULL      NULL         NULL             NULL                    NULL                                NULL     {}
--
-- Orders, read through the lateral join as order_count and last_order_at:
-- Ann 3 (last 2026-10-27 09:00), Bob 1 (2026-10-26 18:30), Cara 2
-- (2026-10-01 11:00), Eve 1 (2026-10-27 09:50), Hana 4 (2026-10-27 08:00),
-- Jade 1 (2025-12-31 23:30), and Nora 2. Everyone else has none.

INSERT INTO countries (id, name, code) VALUES
(1, 'France', 'FR'),
(2, 'Japan', 'JP'),
(3, 'Germany', 'DE'),
(4, 'Brazil', 'BR'),
(5, 'Canada', 'CA');

INSERT INTO people (id, uid, name, nickname, age, height, balance, active, plan, due, last_seen, created_at, country_id, deleted_at,
                    tags, lucky_numbers, ratings, prices, answers, holidays, logins, friend_ids, roles) VALUES
(1, '00000000-0000-4000-8000-000000000001', 'Ann Lee', 'Annie', 34, 1.68, 1250.00, true, 'pro',
 '2026-10-27', '2026-10-27 10:00:00+00', '2024-03-15 09:00:00+00', 1, NULL,
 '{go,sql}', '{7,13}', '{4.5,5}', '{9.99,19.90}', '{true,true}', '{2026-10-27,2026-12-24}',
 '{"2026-10-26 08:00:00+00","2026-10-27 10:00:00+00"}',
 '{00000000-0000-4000-8000-000000000002,00000000-0000-4000-8000-000000000003}', '{admin}'),
(2, '00000000-0000-4000-8000-000000000002', 'Bob Stone', NULL, 41, 1.82, 0.00, false, 'free',
 '2026-10-26', '2026-10-27 09:45:00+00', '2025-01-10 14:30:00+00', 2, NULL,
 '{Go}', '{7}', '{3}', '{120.00}', '{false}', '{2026-11-02}',
 '{"2026-10-27 09:45:00+00"}',
 '{00000000-0000-4000-8000-000000000001}', '{editor}'),
(3, '00000000-0000-4000-8000-000000000003', 'Cara Diaz', 'Cee', 29, 1.6, -35.50, true, 'team',
 '2026-11-01', '2026-10-27 08:30:00+00', '2025-06-01 08:00:00+00', 4, NULL,
 '{rust,go}', '{3,5,8}', '{2.5,4}', '{0.50}', '{true,false}', '{2026-01-01}',
 '{"2026-10-20 12:00:00+00","2026-10-27 08:30:00+00"}',
 '{00000000-0000-4000-8000-000000000001,00000000-0000-4000-8000-000000000008}', '{editor,viewer}'),
(4, '00000000-0000-4000-8000-000000000004', 'Dan O''Brien', NULL, 52, NULL, 980.25, NULL, 'free',
 '2026-10-01', '2026-10-27 00:00:00+00', '2023-11-20 17:45:00+00', 3, NULL,
 '{}', '{}', '{}', '{}', '{}', '{}', '{}', '{}', '{}'),
(5, '00000000-0000-4000-8000-000000000005', 'Eve Martin', 'Evie', 23, 1.75, 15, true, 'pro',
 '2026-10-31', '2026-10-26 23:59:59+00', '2026-02-14 12:00:00+00', 1, NULL,
 NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '{viewer}'),
(6, '00000000-0000-4000-8000-000000000006', 'Finn Larsen', NULL, 67, 1.91, NULL, false, 'team',
 '2026-01-01', '2026-10-26 00:00:00+00', '2022-07-04 10:00:00+00', 3, NULL,
 '{sql}', '{42}', '{1}', '{5.00,5}', '{false,false}', '{2026-12-25}',
 '{"2026-10-26 00:00:00+00"}',
 '{00000000-0000-4000-8000-000000000007}', '{viewer}'),
(7, '00000000-0000-4000-8000-000000000007', 'Gil Moreau', NULL, 38, 1.7, 300.00, NULL, 'free',
 '2026-12-31', '2026-10-25 23:59:59+00', '2024-08-08 16:20:00+00', NULL, NULL,
 '{travel}', '{13}', '{3.5}', '{}', '{true}', '{2026-10-31}',
 NULL,
 '{00000000-0000-4000-8000-000000000006}', '{}'),
(8, '00000000-0000-4000-8000-000000000008', 'Hana Sato', 'Hana', 45, 1.58, 2200.10, true, 'team',
 '2026-10-30', '2026-10-01 00:00:00+00', '2025-03-03 07:15:00+00', 2, NULL,
 '{go,python,sql}', '{1,2,3}', '{5,5,4.5}', '{99.95}', '{true,true,true}', '{2026-10-30,2026-11-01}',
 '{"2026-09-15 09:00:00+00","2026-10-01 00:00:00+00"}',
 '{00000000-0000-4000-8000-000000000001,00000000-0000-4000-8000-000000000003,00000000-0000-4000-8000-000000000010}',
 '{admin,editor}'),
(9, '00000000-0000-4000-8000-000000000009', 'Ivan Petrov', NULL, 17, 1.77, 5.05, true, 'free',
 '2026-11-03', '2026-09-30 23:59:59+00', '2026-09-01 18:00:00+00', 3, NULL,
 '{python}', '{17}', NULL, NULL, '{false,true}', NULL,
 '{"2026-09-30 23:59:59+00"}',
 NULL, '{viewer}'),
(10, '00000000-0000-4000-8000-000000000010', 'Jade Wong', 'JW', 30, 1.65, 42.00, false, 'pro',
 '2025-12-31', '2026-01-01 00:00:00+00', '2025-12-31 23:00:00+00', 2, NULL,
 '{design,go}', '{8}', '{4}', '{12.00}', '{true}', '{2025-12-31}',
 '{"2026-01-01 00:00:00+00"}',
 '{00000000-0000-4000-8000-000000000008}', '{editor}'),
(11, '00000000-0000-4000-8000-000000000011', 'Kai Becker', NULL, -1, 1.8, 0.10, true, 'free',
 '2027-01-01', '2025-12-31 23:59:59+00', '2026-10-20 11:00:00+00', 3, NULL,
 '{}', NULL, '{}', NULL, '{}', '{}',
 NULL,
 '{}', '{viewer}'),
(12, '00000000-0000-4000-8000-000000000012', 'Lena Novak', 'Len', 61, 1.72, 10000.00, false, 'pro',
 '2026-11-02', NULL, '2026-10-27 09:30:00+00', 4, NULL,
 '{sql,design}', '{99}', '{2}', '{1000.00}', '{false}', '{2027-01-01}',
 '{}',
 '{00000000-0000-4000-8000-000000000001,00000000-0000-4000-8000-000000000002}', '{admin}'),
(13, '00000000-0000-4000-8000-000000000013', 'Mo Reyes', NULL, NULL, NULL, NULL, NULL, 'free',
 NULL, NULL, '2026-05-05 12:00:00+00', NULL, NULL,
 NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, '{}'),
(14, '00000000-0000-4000-8000-000000000014', 'Nora Quinn', 'Nora', 34, 1.68, 1250.00, true, 'pro',
 '2026-10-27', '2026-10-27 10:00:00+00', '2025-05-05 09:00:00+00', 1, '2026-10-01 00:00:00+00',
 '{go}', '{7}', '{5}', '{1.00}', '{true}', '{2026-10-27}',
 '{"2026-10-27 10:00:00+00"}',
 '{00000000-0000-4000-8000-000000000001}', '{admin}');

INSERT INTO orders (id, person_id, placed_at) VALUES
(1, 1, '2026-09-01 12:00:00+00'),
(2, 1, '2026-10-20 15:00:00+00'),
(3, 1, '2026-10-27 09:00:00+00'),
(4, 2, '2026-10-26 18:30:00+00'),
(5, 3, '2026-08-15 10:00:00+00'),
(6, 3, '2026-10-01 11:00:00+00'),
(7, 5, '2026-10-27 09:50:00+00'),
(8, 8, '2026-03-01 10:00:00+00'),
(9, 8, '2026-06-01 10:00:00+00'),
(10, 8, '2026-09-01 10:00:00+00'),
(11, 8, '2026-10-27 08:00:00+00'),
(12, 10, '2025-12-31 23:30:00+00'),
(13, 14, '2026-10-27 09:30:00+00'),
(14, 14, '2026-10-27 09:40:00+00');
