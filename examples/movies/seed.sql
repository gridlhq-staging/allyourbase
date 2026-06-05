-- Deterministic movies corpus seed.
-- Re-runnable: upserts by slug so repeated loads never duplicate rows.
-- State-idempotent: the conflict update path only fires when at least one
-- seeded business field actually differs from the row already in the table.
-- Reapplying an unchanged seed leaves `updated_at` (and the row body) intact.

INSERT INTO movies (id, slug, title, overview, release_year, genres, primary_genre, embedding)
VALUES
  (
    '11111111-1111-1111-1111-111111111111',
    'inception',
    'Inception',
    'A thief enters dreams to steal secrets and perform a final heist inside layered realities.',
    2010,
    ARRAY['sci-fi', 'thriller'],
    'Sci-Fi',
    '[0.91,0.12,0.18]'
  ),
  (
    '22222222-2222-2222-2222-222222222222',
    'arrival',
    'Arrival',
    'A linguist helps decode alien language after mysterious ships appear around the world.',
    2016,
    ARRAY['sci-fi', 'drama'],
    'Sci-Fi',
    '[0.31,0.88,0.22]'
  ),
  (
    '33333333-3333-3333-3333-333333333333',
    'moonlight',
    'Moonlight',
    'A young man navigates identity, family, and belonging across three defining chapters of life.',
    2016,
    ARRAY['drama'],
    'Drama',
    '[0.06,0.26,0.97]'
  ),
  (
    '956a520a-ba43-5cc0-9a9a-84997b142c01',
    'showcase-movie-001',
    'Showcase Movie 001',
    'A deterministic high-stakes chase in a coastal city where archive clue 001 connects family choices, public stakes, and a precise release-year signal.',
    1982,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    '495d3d2d-8ac3-5d8c-be66-508e235a283a',
    'showcase-movie-002',
    'Showcase Movie 002',
    'A deterministic expedition in an orbital station where archive clue 002 connects family choices, public stakes, and a precise release-year signal.',
    1989,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    'cbfd3805-f34d-5dd7-aa06-9564cd2cae8c',
    'showcase-movie-003',
    'Showcase Movie 003',
    'A deterministic handcrafted world in a mountain town where archive clue 003 connects family choices, public stakes, and a precise release-year signal.',
    1996,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '25aef713-3759-5ce6-b8c9-35f32581be5f',
    'showcase-movie-004',
    'Showcase Movie 004',
    'A deterministic mistaken-identity romp in a desert highway where archive clue 004 connects family choices, public stakes, and a precise release-year signal.',
    2003,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    'ccc69116-eadc-5023-84d1-05961751a001',
    'showcase-movie-005',
    'Showcase Movie 005',
    'A deterministic archival investigation in a rainy capital where archive clue 005 connects family choices, public stakes, and a precise release-year signal.',
    2010,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    '587c1821-8630-5f55-bd02-67f97f8a8f12',
    'showcase-movie-006',
    'Showcase Movie 006',
    'A deterministic personal turning point in a remote island where archive clue 006 connects family choices, public stakes, and a precise release-year signal.',
    2017,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    '22066336-a421-5e72-953a-b8c1fc0e45b8',
    'showcase-movie-007',
    'Showcase Movie 007',
    'A deterministic mythic quest in an underground archive where archive clue 007 connects family choices, public stakes, and a precise release-year signal.',
    2024,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    '5922c207-9e02-5804-8d97-6fc5685614d9',
    'showcase-movie-008',
    'Showcase Movie 008',
    'A deterministic haunted mystery in a festival weekend where archive clue 008 connects family choices, public stakes, and a precise release-year signal.',
    1980,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    '525fb4a2-8069-552d-9a8d-1f329e6ce02d',
    'showcase-movie-009',
    'Showcase Movie 009',
    'A deterministic cold-case puzzle in a coastal city where archive clue 009 connects family choices, public stakes, and a precise release-year signal.',
    1987,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    'da122e79-cfae-558a-9da2-4c5a01cc6782',
    'showcase-movie-010',
    'Showcase Movie 010',
    'A deterministic second-chance relationship in an orbital station where archive clue 010 connects family choices, public stakes, and a precise release-year signal.',
    1994,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '635ea777-a378-5380-a69f-c7afdc3157bc',
    'showcase-movie-011',
    'Showcase Movie 011',
    'A deterministic future technology dilemma in a mountain town where archive clue 011 connects family choices, public stakes, and a precise release-year signal.',
    2001,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '2ae844e7-ae09-5398-820d-1bb697fed7e2',
    'showcase-movie-012',
    'Showcase Movie 012',
    'A deterministic race against time in a desert highway where archive clue 012 connects family choices, public stakes, and a precise release-year signal.',
    2008,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    '8873b720-f0dc-56e2-9b05-6e56e513dd4f',
    'showcase-movie-013',
    'Showcase Movie 013',
    'A deterministic high-stakes chase in a rainy capital where archive clue 013 connects family choices, public stakes, and a precise release-year signal.',
    2015,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    '0590b01e-440c-5d0a-bd4d-5108201c0ab0',
    'showcase-movie-014',
    'Showcase Movie 014',
    'A deterministic expedition in a remote island where archive clue 014 connects family choices, public stakes, and a precise release-year signal.',
    2022,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '158790b6-3be2-5e8c-bf64-7f6dca45195b',
    'showcase-movie-015',
    'Showcase Movie 015',
    'A deterministic handcrafted world in an underground archive where archive clue 015 connects family choices, public stakes, and a precise release-year signal.',
    1978,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '2a848ad4-dee1-544f-b595-e2e54002fccf',
    'showcase-movie-016',
    'Showcase Movie 016',
    'A deterministic mistaken-identity romp in a festival weekend where archive clue 016 connects family choices, public stakes, and a precise release-year signal.',
    1985,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    '0589ae5b-52f1-5268-b636-e7514fccda89',
    'showcase-movie-017',
    'Showcase Movie 017',
    'A deterministic archival investigation in a coastal city where archive clue 017 connects family choices, public stakes, and a precise release-year signal.',
    1992,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    'd5e172e5-89f9-56f1-9374-c86a4e596c6e',
    'showcase-movie-018',
    'Showcase Movie 018',
    'A deterministic personal turning point in an orbital station where archive clue 018 connects family choices, public stakes, and a precise release-year signal.',
    1999,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    'e3c4d86e-4032-57ad-8e79-c754a67dfe27',
    'showcase-movie-019',
    'Showcase Movie 019',
    'A deterministic mythic quest in a mountain town where archive clue 019 connects family choices, public stakes, and a precise release-year signal.',
    2006,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    '0c8045a5-0d9e-5d92-88c3-6401a477d25c',
    'showcase-movie-020',
    'Showcase Movie 020',
    'A deterministic haunted mystery in a desert highway where archive clue 020 connects family choices, public stakes, and a precise release-year signal.',
    2013,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    'f370a86c-2d84-50bf-9fd2-e8be9fbfe2a1',
    'showcase-movie-021',
    'Showcase Movie 021',
    'A deterministic cold-case puzzle in a rainy capital where archive clue 021 connects family choices, public stakes, and a precise release-year signal.',
    2020,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    '7dfb9257-fca9-58cb-84ba-76e556cfdc81',
    'showcase-movie-022',
    'Showcase Movie 022',
    'A deterministic second-chance relationship in a remote island where archive clue 022 connects family choices, public stakes, and a precise release-year signal.',
    1976,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '93dcd395-493f-5eff-a6b8-5164d7559e5c',
    'showcase-movie-023',
    'Showcase Movie 023',
    'A deterministic future technology dilemma in an underground archive where archive clue 023 connects family choices, public stakes, and a precise release-year signal.',
    1983,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    'b8daf046-85df-525e-908d-c37600856d41',
    'showcase-movie-024',
    'Showcase Movie 024',
    'A deterministic race against time in a festival weekend where archive clue 024 connects family choices, public stakes, and a precise release-year signal.',
    1990,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    '4dac7399-46c9-5f1f-aac5-895c71fa7f38',
    'showcase-movie-025',
    'Showcase Movie 025',
    'A deterministic high-stakes chase in a coastal city where archive clue 025 connects family choices, public stakes, and a precise release-year signal.',
    1997,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    'fafbaec0-8d14-5dce-8d1a-7f20bba3ae89',
    'showcase-movie-026',
    'Showcase Movie 026',
    'A deterministic expedition in an orbital station where archive clue 026 connects family choices, public stakes, and a precise release-year signal.',
    2004,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '110087d1-d8de-50d2-a815-7d282b393cee',
    'showcase-movie-027',
    'Showcase Movie 027',
    'A deterministic handcrafted world in a mountain town where archive clue 027 connects family choices, public stakes, and a precise release-year signal.',
    2011,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '028f5842-9743-51d8-8497-faa74afd716b',
    'showcase-movie-028',
    'Showcase Movie 028',
    'A deterministic mistaken-identity romp in a desert highway where archive clue 028 connects family choices, public stakes, and a precise release-year signal.',
    2018,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    '116f14f2-ecfd-517b-9de2-c7e10ffa0a73',
    'showcase-movie-029',
    'Showcase Movie 029',
    'A deterministic archival investigation in a rainy capital where archive clue 029 connects family choices, public stakes, and a precise release-year signal.',
    2025,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    '3b669626-e15e-5d48-b007-dccae414fe67',
    'showcase-movie-030',
    'Showcase Movie 030',
    'A deterministic personal turning point in a remote island where archive clue 030 connects family choices, public stakes, and a precise release-year signal.',
    1981,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    'cced926d-a622-5203-b876-cf8b95fa9590',
    'showcase-movie-031',
    'Showcase Movie 031',
    'A deterministic mythic quest in an underground archive where archive clue 031 connects family choices, public stakes, and a precise release-year signal.',
    1988,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    '02ed5613-c55e-5310-8b8d-4b766d41a47c',
    'showcase-movie-032',
    'Showcase Movie 032',
    'A deterministic haunted mystery in a festival weekend where archive clue 032 connects family choices, public stakes, and a precise release-year signal.',
    1995,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    '4e59d818-82a1-5037-8ec6-c58c03502662',
    'showcase-movie-033',
    'Showcase Movie 033',
    'A deterministic cold-case puzzle in a coastal city where archive clue 033 connects family choices, public stakes, and a precise release-year signal.',
    2002,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    'b7004628-f705-5f2f-ad72-dcc0f98f4315',
    'showcase-movie-034',
    'Showcase Movie 034',
    'A deterministic second-chance relationship in an orbital station where archive clue 034 connects family choices, public stakes, and a precise release-year signal.',
    2009,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '1993e7c7-7ec8-5187-a2c6-a34a6b5a92a7',
    'showcase-movie-035',
    'Showcase Movie 035',
    'A deterministic future technology dilemma in a mountain town where archive clue 035 connects family choices, public stakes, and a precise release-year signal.',
    2016,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '8b9109cf-3c97-5cfb-b057-a5cffe78188f',
    'showcase-movie-036',
    'Showcase Movie 036',
    'A deterministic race against time in a desert highway where archive clue 036 connects family choices, public stakes, and a precise release-year signal.',
    2023,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    '48a37fe7-9ce5-589f-8745-013d8b92ae35',
    'showcase-movie-037',
    'Showcase Movie 037',
    'A deterministic high-stakes chase in a rainy capital where archive clue 037 connects family choices, public stakes, and a precise release-year signal.',
    1979,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    '3011a092-f4cb-586d-84c3-2c32c5c8596e',
    'showcase-movie-038',
    'Showcase Movie 038',
    'A deterministic expedition in a remote island where archive clue 038 connects family choices, public stakes, and a precise release-year signal.',
    1986,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    'a8300e22-8dde-5eb7-8bb2-7f57d016b1a5',
    'showcase-movie-039',
    'Showcase Movie 039',
    'A deterministic handcrafted world in an underground archive where archive clue 039 connects family choices, public stakes, and a precise release-year signal.',
    1993,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    'aa62817b-f5f4-5538-aae3-43efac886af9',
    'showcase-movie-040',
    'Showcase Movie 040',
    'A deterministic mistaken-identity romp in a festival weekend where archive clue 040 connects family choices, public stakes, and a precise release-year signal.',
    2000,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    'fec17216-5c08-59a6-88f1-a030311a8571',
    'showcase-movie-041',
    'Showcase Movie 041',
    'A deterministic archival investigation in a coastal city where archive clue 041 connects family choices, public stakes, and a precise release-year signal.',
    2007,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    'cf2f1558-c17b-5185-98af-0ce9f54fff42',
    'showcase-movie-042',
    'Showcase Movie 042',
    'A deterministic personal turning point in an orbital station where archive clue 042 connects family choices, public stakes, and a precise release-year signal.',
    2014,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    'e7030a8e-941e-5577-b2e9-57aa1b1abcae',
    'showcase-movie-043',
    'Showcase Movie 043',
    'A deterministic mythic quest in a mountain town where archive clue 043 connects family choices, public stakes, and a precise release-year signal.',
    2021,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    'e9bc701e-6c85-57a8-8b49-6a0dd10bdb25',
    'showcase-movie-044',
    'Showcase Movie 044',
    'A deterministic haunted mystery in a desert highway where archive clue 044 connects family choices, public stakes, and a precise release-year signal.',
    1977,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    'dcfa8af8-7e63-5ed2-b1db-8731a95ff9f0',
    'showcase-movie-045',
    'Showcase Movie 045',
    'A deterministic cold-case puzzle in a rainy capital where archive clue 045 connects family choices, public stakes, and a precise release-year signal.',
    1984,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    '6c0f03a0-02d6-59f9-9842-0f9b8dc90302',
    'showcase-movie-046',
    'Showcase Movie 046',
    'A deterministic second-chance relationship in a remote island where archive clue 046 connects family choices, public stakes, and a precise release-year signal.',
    1991,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    'b6db6624-db91-5677-b865-b54c1c101dc8',
    'showcase-movie-047',
    'Showcase Movie 047',
    'A deterministic future technology dilemma in an underground archive where archive clue 047 connects family choices, public stakes, and a precise release-year signal.',
    1998,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '859d6749-01ef-5459-baf6-f377700508f5',
    'showcase-movie-048',
    'Showcase Movie 048',
    'A deterministic race against time in a festival weekend where archive clue 048 connects family choices, public stakes, and a precise release-year signal.',
    2005,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    'ef3c3600-42e0-57c1-9550-a0d4aa6817b3',
    'showcase-movie-049',
    'Showcase Movie 049',
    'A deterministic high-stakes chase in a coastal city where archive clue 049 connects family choices, public stakes, and a precise release-year signal.',
    2012,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    'ed0a4043-3a5d-5ee3-ba17-89efb9d4e7d6',
    'showcase-movie-050',
    'Showcase Movie 050',
    'A deterministic expedition in an orbital station where archive clue 050 connects family choices, public stakes, and a precise release-year signal.',
    2019,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '67c81ba7-709b-5ee7-b6e0-c62d6ffa60a3',
    'showcase-movie-051',
    'Showcase Movie 051',
    'A deterministic handcrafted world in a mountain town where archive clue 051 connects family choices, public stakes, and a precise release-year signal.',
    1975,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '40bc493f-e4cb-5d30-be82-7152b5335a60',
    'showcase-movie-052',
    'Showcase Movie 052',
    'A deterministic mistaken-identity romp in a desert highway where archive clue 052 connects family choices, public stakes, and a precise release-year signal.',
    1982,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    'f0601718-1dc5-5fef-93ae-bdc9b557045f',
    'showcase-movie-053',
    'Showcase Movie 053',
    'A deterministic archival investigation in a rainy capital where archive clue 053 connects family choices, public stakes, and a precise release-year signal.',
    1989,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    '6778af47-6366-5f78-ab73-1a7f8e7c5876',
    'showcase-movie-054',
    'Showcase Movie 054',
    'A deterministic personal turning point in a remote island where archive clue 054 connects family choices, public stakes, and a precise release-year signal.',
    1996,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    '8fa0ad81-b9d5-53ca-99f5-d4aa3bb9e0b9',
    'showcase-movie-055',
    'Showcase Movie 055',
    'A deterministic mythic quest in an underground archive where archive clue 055 connects family choices, public stakes, and a precise release-year signal.',
    2003,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    '6ae34871-1e98-51f7-9204-b49d346850fb',
    'showcase-movie-056',
    'Showcase Movie 056',
    'A deterministic haunted mystery in a festival weekend where archive clue 056 connects family choices, public stakes, and a precise release-year signal.',
    2010,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    '09ba6964-886a-5a96-9d1e-9d219741577d',
    'showcase-movie-057',
    'Showcase Movie 057',
    'A deterministic cold-case puzzle in a coastal city where archive clue 057 connects family choices, public stakes, and a precise release-year signal.',
    2017,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    'a633cbbb-6e72-501b-8883-20108ea6d0c7',
    'showcase-movie-058',
    'Showcase Movie 058',
    'A deterministic second-chance relationship in an orbital station where archive clue 058 connects family choices, public stakes, and a precise release-year signal.',
    2024,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    'be481929-5622-5f1c-ae10-b640901f2679',
    'showcase-movie-059',
    'Showcase Movie 059',
    'A deterministic future technology dilemma in a mountain town where archive clue 059 connects family choices, public stakes, and a precise release-year signal.',
    1980,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '22916c8a-883f-5332-b020-98d810193e05',
    'showcase-movie-060',
    'Showcase Movie 060',
    'A deterministic race against time in a desert highway where archive clue 060 connects family choices, public stakes, and a precise release-year signal.',
    1987,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    '6903479d-3a06-5e2f-b8fc-a1a19e674104',
    'showcase-movie-061',
    'Showcase Movie 061',
    'A deterministic high-stakes chase in a rainy capital where archive clue 061 connects family choices, public stakes, and a precise release-year signal.',
    1994,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    'a4c7defe-4fa1-5ecd-b8f1-332d577cec91',
    'showcase-movie-062',
    'Showcase Movie 062',
    'A deterministic expedition in a remote island where archive clue 062 connects family choices, public stakes, and a precise release-year signal.',
    2001,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '31f74f73-b25a-5812-8306-e7056851ae46',
    'showcase-movie-063',
    'Showcase Movie 063',
    'A deterministic handcrafted world in an underground archive where archive clue 063 connects family choices, public stakes, and a precise release-year signal.',
    2008,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '1e10b6bd-1b5b-55cc-b049-7f554b845205',
    'showcase-movie-064',
    'Showcase Movie 064',
    'A deterministic mistaken-identity romp in a festival weekend where archive clue 064 connects family choices, public stakes, and a precise release-year signal.',
    2015,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    '330dea18-8241-5c3a-a13a-d339dcd010aa',
    'showcase-movie-065',
    'Showcase Movie 065',
    'A deterministic archival investigation in a coastal city where archive clue 065 connects family choices, public stakes, and a precise release-year signal.',
    2022,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    '875307b9-8d08-55b8-83b7-499a918a4771',
    'showcase-movie-066',
    'Showcase Movie 066',
    'A deterministic personal turning point in an orbital station where archive clue 066 connects family choices, public stakes, and a precise release-year signal.',
    1978,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    '394eaeed-8647-5d3f-93e0-c4d8a835b4fe',
    'showcase-movie-067',
    'Showcase Movie 067',
    'A deterministic mythic quest in a mountain town where archive clue 067 connects family choices, public stakes, and a precise release-year signal.',
    1985,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    'f172e302-45de-55a4-ab1b-e9025d0ebdfd',
    'showcase-movie-068',
    'Showcase Movie 068',
    'A deterministic haunted mystery in a desert highway where archive clue 068 connects family choices, public stakes, and a precise release-year signal.',
    1992,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    'a1b36a9f-69a1-578e-b28c-4daabe20f14f',
    'showcase-movie-069',
    'Showcase Movie 069',
    'A deterministic cold-case puzzle in a rainy capital where archive clue 069 connects family choices, public stakes, and a precise release-year signal.',
    1999,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    'b529962c-f90c-5004-b1fb-c3705b5149e5',
    'showcase-movie-070',
    'Showcase Movie 070',
    'A deterministic second-chance relationship in a remote island where archive clue 070 connects family choices, public stakes, and a precise release-year signal.',
    2006,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '233595d7-885a-51ba-b22f-41b1b12ea5bd',
    'showcase-movie-071',
    'Showcase Movie 071',
    'A deterministic future technology dilemma in an underground archive where archive clue 071 connects family choices, public stakes, and a precise release-year signal.',
    2013,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '436b52cd-4062-5db7-b6d8-a438a0e4a832',
    'showcase-movie-072',
    'Showcase Movie 072',
    'A deterministic race against time in a festival weekend where archive clue 072 connects family choices, public stakes, and a precise release-year signal.',
    2020,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    '3ccd73f9-a7ab-5fcc-b2c3-02127d88088a',
    'showcase-movie-073',
    'Showcase Movie 073',
    'A deterministic high-stakes chase in a coastal city where archive clue 073 connects family choices, public stakes, and a precise release-year signal.',
    1976,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    '1e8a74e9-8154-542a-ac9b-ee5e38aefacc',
    'showcase-movie-074',
    'Showcase Movie 074',
    'A deterministic expedition in an orbital station where archive clue 074 connects family choices, public stakes, and a precise release-year signal.',
    1983,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '1965ca60-4896-59fd-a845-dc648f02cfe5',
    'showcase-movie-075',
    'Showcase Movie 075',
    'A deterministic handcrafted world in a mountain town where archive clue 075 connects family choices, public stakes, and a precise release-year signal.',
    1990,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    'de9729f3-fcc7-5bd0-97f7-b10a97190546',
    'showcase-movie-076',
    'Showcase Movie 076',
    'A deterministic mistaken-identity romp in a desert highway where archive clue 076 connects family choices, public stakes, and a precise release-year signal.',
    1997,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    'fe5641d3-a320-5b54-9eeb-9fe45e6e041a',
    'showcase-movie-077',
    'Showcase Movie 077',
    'A deterministic archival investigation in a rainy capital where archive clue 077 connects family choices, public stakes, and a precise release-year signal.',
    2004,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    '594f742d-ca8e-556c-b12c-3aaaf6e66e6a',
    'showcase-movie-078',
    'Showcase Movie 078',
    'A deterministic personal turning point in a remote island where archive clue 078 connects family choices, public stakes, and a precise release-year signal.',
    2011,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    '78532cd6-86e4-5c92-ad90-36790b28ec2a',
    'showcase-movie-079',
    'Showcase Movie 079',
    'A deterministic mythic quest in an underground archive where archive clue 079 connects family choices, public stakes, and a precise release-year signal.',
    2018,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    '18604d7f-8ba0-5cda-ba1b-9d009eae9ea5',
    'showcase-movie-080',
    'Showcase Movie 080',
    'A deterministic haunted mystery in a festival weekend where archive clue 080 connects family choices, public stakes, and a precise release-year signal.',
    2025,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    '883d15b5-d93c-5465-b7a2-6eca4310246f',
    'showcase-movie-081',
    'Showcase Movie 081',
    'A deterministic cold-case puzzle in a coastal city where archive clue 081 connects family choices, public stakes, and a precise release-year signal.',
    1981,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    '9a57e97b-a5e8-5c92-9885-29cde40aeb55',
    'showcase-movie-082',
    'Showcase Movie 082',
    'A deterministic second-chance relationship in an orbital station where archive clue 082 connects family choices, public stakes, and a precise release-year signal.',
    1988,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '172f8509-4045-5a4b-9c31-383c1a1eae22',
    'showcase-movie-083',
    'Showcase Movie 083',
    'A deterministic future technology dilemma in a mountain town where archive clue 083 connects family choices, public stakes, and a precise release-year signal.',
    1995,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '08c54f0e-3c93-59ad-a4ae-0ac3bfa4931c',
    'showcase-movie-084',
    'Showcase Movie 084',
    'A deterministic race against time in a desert highway where archive clue 084 connects family choices, public stakes, and a precise release-year signal.',
    2002,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    '8ab55f8f-da35-5acf-95ae-0b551c724124',
    'showcase-movie-085',
    'Showcase Movie 085',
    'A deterministic high-stakes chase in a rainy capital where archive clue 085 connects family choices, public stakes, and a precise release-year signal.',
    2009,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    '05811490-3a14-56e0-a55c-7c23a3fbe004',
    'showcase-movie-086',
    'Showcase Movie 086',
    'A deterministic expedition in a remote island where archive clue 086 connects family choices, public stakes, and a precise release-year signal.',
    2016,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    'ef1f4544-e674-561a-9331-628545bff166',
    'showcase-movie-087',
    'Showcase Movie 087',
    'A deterministic handcrafted world in an underground archive where archive clue 087 connects family choices, public stakes, and a precise release-year signal.',
    2023,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    'c9e64b62-6b50-569a-a70a-7c01f6115cfe',
    'showcase-movie-088',
    'Showcase Movie 088',
    'A deterministic mistaken-identity romp in a festival weekend where archive clue 088 connects family choices, public stakes, and a precise release-year signal.',
    1979,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    '4daf90e3-f319-5509-9060-369ac8f81f63',
    'showcase-movie-089',
    'Showcase Movie 089',
    'A deterministic archival investigation in a coastal city where archive clue 089 connects family choices, public stakes, and a precise release-year signal.',
    1986,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    '2a91f59c-7602-57e5-be9f-3d3a77359f3d',
    'showcase-movie-090',
    'Showcase Movie 090',
    'A deterministic personal turning point in an orbital station where archive clue 090 connects family choices, public stakes, and a precise release-year signal.',
    1993,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    'e1f7407d-d1fd-5b1d-98e7-2ccc155e50bd',
    'showcase-movie-091',
    'Showcase Movie 091',
    'A deterministic mythic quest in a mountain town where archive clue 091 connects family choices, public stakes, and a precise release-year signal.',
    2000,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    'f43fb901-1743-5621-b6a9-cfe7018ebd35',
    'showcase-movie-092',
    'Showcase Movie 092',
    'A deterministic haunted mystery in a desert highway where archive clue 092 connects family choices, public stakes, and a precise release-year signal.',
    2007,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    '43fba617-4dfd-5aae-a7eb-7b5f156be6f1',
    'showcase-movie-093',
    'Showcase Movie 093',
    'A deterministic cold-case puzzle in a rainy capital where archive clue 093 connects family choices, public stakes, and a precise release-year signal.',
    2014,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    'a702fe41-7930-5346-a2b7-9dd99a2c7fb2',
    'showcase-movie-094',
    'Showcase Movie 094',
    'A deterministic second-chance relationship in a remote island where archive clue 094 connects family choices, public stakes, and a precise release-year signal.',
    2021,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    'cb53c655-55b0-5ff6-bbd0-912998e73149',
    'showcase-movie-095',
    'Showcase Movie 095',
    'A deterministic future technology dilemma in an underground archive where archive clue 095 connects family choices, public stakes, and a precise release-year signal.',
    1977,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    'da121f72-e2ca-5f5a-a6f9-df23bc0f3150',
    'showcase-movie-096',
    'Showcase Movie 096',
    'A deterministic race against time in a festival weekend where archive clue 096 connects family choices, public stakes, and a precise release-year signal.',
    1984,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    'f9d119c7-8f74-5f12-bb76-1abcbb30f584',
    'showcase-movie-097',
    'Showcase Movie 097',
    'A deterministic high-stakes chase in a coastal city where archive clue 097 connects family choices, public stakes, and a precise release-year signal.',
    1991,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    '8ae6c602-9981-5190-a825-dbcd3a18da77',
    'showcase-movie-098',
    'Showcase Movie 098',
    'A deterministic expedition in an orbital station where archive clue 098 connects family choices, public stakes, and a precise release-year signal.',
    1998,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '5e8c5943-443e-5ed0-b00c-de38c55d6e83',
    'showcase-movie-099',
    'Showcase Movie 099',
    'A deterministic handcrafted world in a mountain town where archive clue 099 connects family choices, public stakes, and a precise release-year signal.',
    2005,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '6581b6f8-4efb-50b1-a22d-9827c7c29494',
    'showcase-movie-100',
    'Showcase Movie 100',
    'A deterministic mistaken-identity romp in a desert highway where archive clue 100 connects family choices, public stakes, and a precise release-year signal.',
    2012,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    '426d352c-3f41-5553-a42c-58c1e4f06982',
    'showcase-movie-101',
    'Showcase Movie 101',
    'A deterministic archival investigation in a rainy capital where archive clue 101 connects family choices, public stakes, and a precise release-year signal.',
    2019,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    'ae0e1577-0f91-5776-8966-f2c0144c2bcb',
    'showcase-movie-102',
    'Showcase Movie 102',
    'A deterministic personal turning point in a remote island where archive clue 102 connects family choices, public stakes, and a precise release-year signal.',
    1975,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    '5961e082-eba7-5165-9cea-5c2f1499927e',
    'showcase-movie-103',
    'Showcase Movie 103',
    'A deterministic mythic quest in an underground archive where archive clue 103 connects family choices, public stakes, and a precise release-year signal.',
    1982,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    '7709cd1f-f8d6-5c6f-aaee-423b4fb90387',
    'showcase-movie-104',
    'Showcase Movie 104',
    'A deterministic haunted mystery in a festival weekend where archive clue 104 connects family choices, public stakes, and a precise release-year signal.',
    1989,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    'c33edb91-ab0b-534b-bd6e-bad9f98be1de',
    'showcase-movie-105',
    'Showcase Movie 105',
    'A deterministic cold-case puzzle in a coastal city where archive clue 105 connects family choices, public stakes, and a precise release-year signal.',
    1996,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    '785ad93a-c539-5e10-a5e6-ac6ddce2d410',
    'showcase-movie-106',
    'Showcase Movie 106',
    'A deterministic second-chance relationship in an orbital station where archive clue 106 connects family choices, public stakes, and a precise release-year signal.',
    2003,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '9e8e20ca-714d-5b0e-9cf2-01aec5e6d505',
    'showcase-movie-107',
    'Showcase Movie 107',
    'A deterministic future technology dilemma in a mountain town where archive clue 107 connects family choices, public stakes, and a precise release-year signal.',
    2010,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '1b221fc1-7b4e-56b4-a635-20dd2cee8b83',
    'showcase-movie-108',
    'Showcase Movie 108',
    'A deterministic race against time in a desert highway where archive clue 108 connects family choices, public stakes, and a precise release-year signal.',
    2017,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    'fdf2fa19-4ca8-5887-ab96-e3a46ee05e44',
    'showcase-movie-109',
    'Showcase Movie 109',
    'A deterministic high-stakes chase in a rainy capital where archive clue 109 connects family choices, public stakes, and a precise release-year signal.',
    2024,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    'bcbfc1cf-8058-545b-a8c4-f89a01bca51f',
    'showcase-movie-110',
    'Showcase Movie 110',
    'A deterministic expedition in a remote island where archive clue 110 connects family choices, public stakes, and a precise release-year signal.',
    1980,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '31639b5a-4bd7-5cee-a11c-6826cc91980c',
    'showcase-movie-111',
    'Showcase Movie 111',
    'A deterministic handcrafted world in an underground archive where archive clue 111 connects family choices, public stakes, and a precise release-year signal.',
    1987,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '5b597a50-35f4-5004-b10f-2855c81246a7',
    'showcase-movie-112',
    'Showcase Movie 112',
    'A deterministic mistaken-identity romp in a festival weekend where archive clue 112 connects family choices, public stakes, and a precise release-year signal.',
    1994,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    '280bfd1a-1703-512a-b1df-05d4adfc63a6',
    'showcase-movie-113',
    'Showcase Movie 113',
    'A deterministic archival investigation in a coastal city where archive clue 113 connects family choices, public stakes, and a precise release-year signal.',
    2001,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    'ab5f4e02-0edf-542c-b197-8e0eac788516',
    'showcase-movie-114',
    'Showcase Movie 114',
    'A deterministic personal turning point in an orbital station where archive clue 114 connects family choices, public stakes, and a precise release-year signal.',
    2008,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    'f8cd58df-4d12-5cf2-a0b3-620992cc3e90',
    'showcase-movie-115',
    'Showcase Movie 115',
    'A deterministic mythic quest in a mountain town where archive clue 115 connects family choices, public stakes, and a precise release-year signal.',
    2015,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    'ea6a6adb-b328-599f-be9a-3a56d001f321',
    'showcase-movie-116',
    'Showcase Movie 116',
    'A deterministic haunted mystery in a desert highway where archive clue 116 connects family choices, public stakes, and a precise release-year signal.',
    2022,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    'f4e6cbdb-e0c5-5383-b9bb-d42e8c599559',
    'showcase-movie-117',
    'Showcase Movie 117',
    'A deterministic cold-case puzzle in a rainy capital where archive clue 117 connects family choices, public stakes, and a precise release-year signal.',
    1978,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    '2219e80e-3d59-5109-a950-5810d823db6d',
    'showcase-movie-118',
    'Showcase Movie 118',
    'A deterministic second-chance relationship in a remote island where archive clue 118 connects family choices, public stakes, and a precise release-year signal.',
    1985,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '38ef8bc8-a7b8-58c7-b075-7376903432a3',
    'showcase-movie-119',
    'Showcase Movie 119',
    'A deterministic future technology dilemma in an underground archive where archive clue 119 connects family choices, public stakes, and a precise release-year signal.',
    1992,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '9b2b215f-df28-5a6e-aa3f-856a5c1814d1',
    'showcase-movie-120',
    'Showcase Movie 120',
    'A deterministic race against time in a festival weekend where archive clue 120 connects family choices, public stakes, and a precise release-year signal.',
    1999,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    '99cc6f2f-39cf-5ca6-929b-3042cb502867',
    'showcase-movie-121',
    'Showcase Movie 121',
    'A deterministic high-stakes chase in a coastal city where archive clue 121 connects family choices, public stakes, and a precise release-year signal.',
    2006,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    'a071bdd2-b3a9-5514-8072-9b56a8fb121e',
    'showcase-movie-122',
    'Showcase Movie 122',
    'A deterministic expedition in an orbital station where archive clue 122 connects family choices, public stakes, and a precise release-year signal.',
    2013,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '1d36897e-743c-533f-b1a4-e3d976a04cfe',
    'showcase-movie-123',
    'Showcase Movie 123',
    'A deterministic handcrafted world in a mountain town where archive clue 123 connects family choices, public stakes, and a precise release-year signal.',
    2020,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '74f4adff-95f4-5582-9f2b-b8e856bc42b2',
    'showcase-movie-124',
    'Showcase Movie 124',
    'A deterministic mistaken-identity romp in a desert highway where archive clue 124 connects family choices, public stakes, and a precise release-year signal.',
    1976,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    'd73b51eb-0ae5-572d-8b5b-c93bafc5cab2',
    'showcase-movie-125',
    'Showcase Movie 125',
    'A deterministic archival investigation in a rainy capital where archive clue 125 connects family choices, public stakes, and a precise release-year signal.',
    1983,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    '3a9fe1c7-1f1c-5d70-abbb-ab7ca678c606',
    'showcase-movie-126',
    'Showcase Movie 126',
    'A deterministic personal turning point in a remote island where archive clue 126 connects family choices, public stakes, and a precise release-year signal.',
    1990,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    'b05562f8-d813-5d7e-85ba-f5c553148eb1',
    'showcase-movie-127',
    'Showcase Movie 127',
    'A deterministic mythic quest in an underground archive where archive clue 127 connects family choices, public stakes, and a precise release-year signal.',
    1997,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    '5c3389b7-a4ab-51a9-a52d-617abb5e7cce',
    'showcase-movie-128',
    'Showcase Movie 128',
    'A deterministic haunted mystery in a festival weekend where archive clue 128 connects family choices, public stakes, and a precise release-year signal.',
    2004,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    '93a54fd7-949d-58bc-a5e5-9f31efcecec5',
    'showcase-movie-129',
    'Showcase Movie 129',
    'A deterministic cold-case puzzle in a coastal city where archive clue 129 connects family choices, public stakes, and a precise release-year signal.',
    2011,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    '3a7e4dd1-88ea-55bc-9cf8-c9db0fd80f36',
    'showcase-movie-130',
    'Showcase Movie 130',
    'A deterministic second-chance relationship in an orbital station where archive clue 130 connects family choices, public stakes, and a precise release-year signal.',
    2018,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '18cf165e-2efb-5f3d-8e19-211731a11506',
    'showcase-movie-131',
    'Showcase Movie 131',
    'A deterministic future technology dilemma in a mountain town where archive clue 131 connects family choices, public stakes, and a precise release-year signal.',
    2025,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '64a32acc-e1b6-5b71-94b4-1c5e40333012',
    'showcase-movie-132',
    'Showcase Movie 132',
    'A deterministic race against time in a desert highway where archive clue 132 connects family choices, public stakes, and a precise release-year signal.',
    1981,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    '921206d4-0c45-5839-a22f-42ec48889352',
    'showcase-movie-133',
    'Showcase Movie 133',
    'A deterministic high-stakes chase in a rainy capital where archive clue 133 connects family choices, public stakes, and a precise release-year signal.',
    1988,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    'e2c08b6f-4cc0-5dd8-8ca2-c93b6a6a573f',
    'showcase-movie-134',
    'Showcase Movie 134',
    'A deterministic expedition in a remote island where archive clue 134 connects family choices, public stakes, and a precise release-year signal.',
    1995,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '05e4c052-5036-5f77-aa01-1c73194c6ff7',
    'showcase-movie-135',
    'Showcase Movie 135',
    'A deterministic handcrafted world in an underground archive where archive clue 135 connects family choices, public stakes, and a precise release-year signal.',
    2002,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    'd8870abb-7c76-5378-85b6-140c2a48b68c',
    'showcase-movie-136',
    'Showcase Movie 136',
    'A deterministic mistaken-identity romp in a festival weekend where archive clue 136 connects family choices, public stakes, and a precise release-year signal.',
    2009,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    '1ad89448-c733-5e27-a893-f4ead31f03b3',
    'showcase-movie-137',
    'Showcase Movie 137',
    'A deterministic archival investigation in a coastal city where archive clue 137 connects family choices, public stakes, and a precise release-year signal.',
    2016,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    '62cddf4e-9295-5c66-83cf-f5a2ce775564',
    'showcase-movie-138',
    'Showcase Movie 138',
    'A deterministic personal turning point in an orbital station where archive clue 138 connects family choices, public stakes, and a precise release-year signal.',
    2023,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    '2864c388-b66c-528b-9878-5cb7f1018581',
    'showcase-movie-139',
    'Showcase Movie 139',
    'A deterministic mythic quest in a mountain town where archive clue 139 connects family choices, public stakes, and a precise release-year signal.',
    1979,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    '99b05c49-3815-5f0c-9ea6-7d74d7a698a9',
    'showcase-movie-140',
    'Showcase Movie 140',
    'A deterministic haunted mystery in a desert highway where archive clue 140 connects family choices, public stakes, and a precise release-year signal.',
    1986,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    'd6fcfaca-496b-5a36-ad6c-47f86d0d5518',
    'showcase-movie-141',
    'Showcase Movie 141',
    'A deterministic cold-case puzzle in a rainy capital where archive clue 141 connects family choices, public stakes, and a precise release-year signal.',
    1993,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    '6e55db42-4b09-527d-b9a5-7449389e1adf',
    'showcase-movie-142',
    'Showcase Movie 142',
    'A deterministic second-chance relationship in a remote island where archive clue 142 connects family choices, public stakes, and a precise release-year signal.',
    2000,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '5dfe6490-0366-59e1-aab5-63ea64c402d6',
    'showcase-movie-143',
    'Showcase Movie 143',
    'A deterministic future technology dilemma in an underground archive where archive clue 143 connects family choices, public stakes, and a precise release-year signal.',
    2007,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '992f8b3f-70ef-5cf8-a968-8ad606be04e0',
    'showcase-movie-144',
    'Showcase Movie 144',
    'A deterministic race against time in a festival weekend where archive clue 144 connects family choices, public stakes, and a precise release-year signal.',
    2014,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    '9f929e74-0ca2-5cd1-8704-aab8dd5089bd',
    'showcase-movie-145',
    'Showcase Movie 145',
    'A deterministic high-stakes chase in a coastal city where archive clue 145 connects family choices, public stakes, and a precise release-year signal.',
    2021,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    '0ff54db5-0b8c-5623-8f36-e7038ff51a4a',
    'showcase-movie-146',
    'Showcase Movie 146',
    'A deterministic expedition in an orbital station where archive clue 146 connects family choices, public stakes, and a precise release-year signal.',
    1977,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '8e31efb7-3b35-5b4e-bb38-534d8f75ae0c',
    'showcase-movie-147',
    'Showcase Movie 147',
    'A deterministic handcrafted world in a mountain town where archive clue 147 connects family choices, public stakes, and a precise release-year signal.',
    1984,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '0783e4a8-0b88-5fd8-9b1e-eff5c6d138de',
    'showcase-movie-148',
    'Showcase Movie 148',
    'A deterministic mistaken-identity romp in a desert highway where archive clue 148 connects family choices, public stakes, and a precise release-year signal.',
    1991,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    'f1a586a5-6868-593a-852f-56e7b3c906d6',
    'showcase-movie-149',
    'Showcase Movie 149',
    'A deterministic archival investigation in a rainy capital where archive clue 149 connects family choices, public stakes, and a precise release-year signal.',
    1998,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    '2b29d6ae-35be-511c-bff5-18f6ba8d0f32',
    'showcase-movie-150',
    'Showcase Movie 150',
    'A deterministic personal turning point in a remote island where archive clue 150 connects family choices, public stakes, and a precise release-year signal.',
    2005,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    '4f0d27fd-f226-5b4d-bbb9-edee0cb3d566',
    'showcase-movie-151',
    'Showcase Movie 151',
    'A deterministic mythic quest in an underground archive where archive clue 151 connects family choices, public stakes, and a precise release-year signal.',
    2012,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    '9ceeec63-15ad-584f-b8bd-82f08ff0362d',
    'showcase-movie-152',
    'Showcase Movie 152',
    'A deterministic haunted mystery in a festival weekend where archive clue 152 connects family choices, public stakes, and a precise release-year signal.',
    2019,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    'c6c656eb-5480-5ef8-ad81-59cb79feb4b7',
    'showcase-movie-153',
    'Showcase Movie 153',
    'A deterministic cold-case puzzle in a coastal city where archive clue 153 connects family choices, public stakes, and a precise release-year signal.',
    1975,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    '097db27f-405c-5b0e-9e64-23d4252e4c00',
    'showcase-movie-154',
    'Showcase Movie 154',
    'A deterministic second-chance relationship in an orbital station where archive clue 154 connects family choices, public stakes, and a precise release-year signal.',
    1982,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '8d1e4b54-f2ce-5e50-a9a4-7236860b91c0',
    'showcase-movie-155',
    'Showcase Movie 155',
    'A deterministic future technology dilemma in a mountain town where archive clue 155 connects family choices, public stakes, and a precise release-year signal.',
    1989,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '8f41c5b0-4a44-589b-8750-d175dc31e4f6',
    'showcase-movie-156',
    'Showcase Movie 156',
    'A deterministic race against time in a desert highway where archive clue 156 connects family choices, public stakes, and a precise release-year signal.',
    1996,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    'e99e4a52-cf3c-5285-b4d1-2b4bd8b5ea26',
    'showcase-movie-157',
    'Showcase Movie 157',
    'A deterministic high-stakes chase in a rainy capital where archive clue 157 connects family choices, public stakes, and a precise release-year signal.',
    2003,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    'c97d0dac-3e6f-594a-a3f5-897aa928c11e',
    'showcase-movie-158',
    'Showcase Movie 158',
    'A deterministic expedition in a remote island where archive clue 158 connects family choices, public stakes, and a precise release-year signal.',
    2010,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '1a4c97db-563e-574d-b821-c32b12451f32',
    'showcase-movie-159',
    'Showcase Movie 159',
    'A deterministic handcrafted world in an underground archive where archive clue 159 connects family choices, public stakes, and a precise release-year signal.',
    2017,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '722fd4e6-e999-5fcd-84da-ee06b9179d1d',
    'showcase-movie-160',
    'Showcase Movie 160',
    'A deterministic mistaken-identity romp in a festival weekend where archive clue 160 connects family choices, public stakes, and a precise release-year signal.',
    2024,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    '6973d88b-8e5c-5653-a055-e67d5456386c',
    'showcase-movie-161',
    'Showcase Movie 161',
    'A deterministic archival investigation in a coastal city where archive clue 161 connects family choices, public stakes, and a precise release-year signal.',
    1980,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    '3b7f82c7-3bd1-584b-af80-5ca1f6d76124',
    'showcase-movie-162',
    'Showcase Movie 162',
    'A deterministic personal turning point in an orbital station where archive clue 162 connects family choices, public stakes, and a precise release-year signal.',
    1987,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    '7837614a-bdaf-554c-b650-a4694eba2bb5',
    'showcase-movie-163',
    'Showcase Movie 163',
    'A deterministic mythic quest in a mountain town where archive clue 163 connects family choices, public stakes, and a precise release-year signal.',
    1994,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    '30362790-ffb6-5691-ab72-ca5703c4ceef',
    'showcase-movie-164',
    'Showcase Movie 164',
    'A deterministic haunted mystery in a desert highway where archive clue 164 connects family choices, public stakes, and a precise release-year signal.',
    2001,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    '4574c5e3-bdd8-5b36-b08c-45d8d48d7c99',
    'showcase-movie-165',
    'Showcase Movie 165',
    'A deterministic cold-case puzzle in a rainy capital where archive clue 165 connects family choices, public stakes, and a precise release-year signal.',
    2008,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    '73f24ba9-f413-5bac-9421-bda72ed954b7',
    'showcase-movie-166',
    'Showcase Movie 166',
    'A deterministic second-chance relationship in a remote island where archive clue 166 connects family choices, public stakes, and a precise release-year signal.',
    2015,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '8dec8584-f44b-5266-b9c1-f8f3b87cccb5',
    'showcase-movie-167',
    'Showcase Movie 167',
    'A deterministic future technology dilemma in an underground archive where archive clue 167 connects family choices, public stakes, and a precise release-year signal.',
    2022,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '45af1671-7616-5d2d-b1c5-85fe36ca363d',
    'showcase-movie-168',
    'Showcase Movie 168',
    'A deterministic race against time in a festival weekend where archive clue 168 connects family choices, public stakes, and a precise release-year signal.',
    1978,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    'b21bc9dd-0189-522b-85ca-480e92b9ac8f',
    'showcase-movie-169',
    'Showcase Movie 169',
    'A deterministic high-stakes chase in a coastal city where archive clue 169 connects family choices, public stakes, and a precise release-year signal.',
    1985,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    '7c8d2c37-1450-5ea9-bfb8-ad0e6c7a1007',
    'showcase-movie-170',
    'Showcase Movie 170',
    'A deterministic expedition in an orbital station where archive clue 170 connects family choices, public stakes, and a precise release-year signal.',
    1992,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '71914a9d-eef5-5585-8418-64a585275d3b',
    'showcase-movie-171',
    'Showcase Movie 171',
    'A deterministic handcrafted world in a mountain town where archive clue 171 connects family choices, public stakes, and a precise release-year signal.',
    1999,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '42206839-06d0-510b-8467-238f57e09c7b',
    'showcase-movie-172',
    'Showcase Movie 172',
    'A deterministic mistaken-identity romp in a desert highway where archive clue 172 connects family choices, public stakes, and a precise release-year signal.',
    2006,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    'd6d4b91d-5deb-5b6b-b9e3-63bf34c8cabd',
    'showcase-movie-173',
    'Showcase Movie 173',
    'A deterministic archival investigation in a rainy capital where archive clue 173 connects family choices, public stakes, and a precise release-year signal.',
    2013,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    '805b95c7-6633-54ee-a4ec-e9f97557e1ce',
    'showcase-movie-174',
    'Showcase Movie 174',
    'A deterministic personal turning point in a remote island where archive clue 174 connects family choices, public stakes, and a precise release-year signal.',
    2020,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    '5dde37ce-e7c9-564a-8758-74bc76160dc1',
    'showcase-movie-175',
    'Showcase Movie 175',
    'A deterministic mythic quest in an underground archive where archive clue 175 connects family choices, public stakes, and a precise release-year signal.',
    1976,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    '5c8fe62f-ac43-56f5-aed0-c9d2f04dd69d',
    'showcase-movie-176',
    'Showcase Movie 176',
    'A deterministic haunted mystery in a festival weekend where archive clue 176 connects family choices, public stakes, and a precise release-year signal.',
    1983,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    'f09856fb-3182-5464-b718-3f3661fd3280',
    'showcase-movie-177',
    'Showcase Movie 177',
    'A deterministic cold-case puzzle in a coastal city where archive clue 177 connects family choices, public stakes, and a precise release-year signal.',
    1990,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    '34e5cd2a-4d11-5d1d-b7ef-298871131c11',
    'showcase-movie-178',
    'Showcase Movie 178',
    'A deterministic second-chance relationship in an orbital station where archive clue 178 connects family choices, public stakes, and a precise release-year signal.',
    1997,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '894154fd-df16-5f28-9315-0133fee1a1c6',
    'showcase-movie-179',
    'Showcase Movie 179',
    'A deterministic future technology dilemma in a mountain town where archive clue 179 connects family choices, public stakes, and a precise release-year signal.',
    2004,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    'c49b1d27-3630-5ec3-b5de-0594102c8d4a',
    'showcase-movie-180',
    'Showcase Movie 180',
    'A deterministic race against time in a desert highway where archive clue 180 connects family choices, public stakes, and a precise release-year signal.',
    2011,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    'a022a338-658b-5a90-b927-72c74882b640',
    'showcase-movie-181',
    'Showcase Movie 181',
    'A deterministic high-stakes chase in a rainy capital where archive clue 181 connects family choices, public stakes, and a precise release-year signal.',
    2018,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    '9ef075d7-e406-5008-a631-75d3ea561050',
    'showcase-movie-182',
    'Showcase Movie 182',
    'A deterministic expedition in a remote island where archive clue 182 connects family choices, public stakes, and a precise release-year signal.',
    2025,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '03919f1b-f11a-5293-a46a-86130121b01c',
    'showcase-movie-183',
    'Showcase Movie 183',
    'A deterministic handcrafted world in an underground archive where archive clue 183 connects family choices, public stakes, and a precise release-year signal.',
    1981,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '8c129755-5487-5902-8704-446da1edab3e',
    'showcase-movie-184',
    'Showcase Movie 184',
    'A deterministic mistaken-identity romp in a festival weekend where archive clue 184 connects family choices, public stakes, and a precise release-year signal.',
    1988,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    'feedc5ce-3cc3-5a31-9e4e-59be835c2031',
    'showcase-movie-185',
    'Showcase Movie 185',
    'A deterministic archival investigation in a coastal city where archive clue 185 connects family choices, public stakes, and a precise release-year signal.',
    1995,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    '61ae3691-a243-5b2e-98a3-8db686e66fa1',
    'showcase-movie-186',
    'Showcase Movie 186',
    'A deterministic personal turning point in an orbital station where archive clue 186 connects family choices, public stakes, and a precise release-year signal.',
    2002,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    '1d5996ae-f6f4-5127-894b-295fd279fa9b',
    'showcase-movie-187',
    'Showcase Movie 187',
    'A deterministic mythic quest in a mountain town where archive clue 187 connects family choices, public stakes, and a precise release-year signal.',
    2009,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    '4ef9f077-cea7-5ea3-b096-73c0d7a68523',
    'showcase-movie-188',
    'Showcase Movie 188',
    'A deterministic haunted mystery in a desert highway where archive clue 188 connects family choices, public stakes, and a precise release-year signal.',
    2016,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    '6493877c-8195-5a0f-8e0d-8bb251dd03ba',
    'showcase-movie-189',
    'Showcase Movie 189',
    'A deterministic cold-case puzzle in a rainy capital where archive clue 189 connects family choices, public stakes, and a precise release-year signal.',
    2023,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    '580c9cf5-1f04-5948-9e77-6b8784df89f2',
    'showcase-movie-190',
    'Showcase Movie 190',
    'A deterministic second-chance relationship in a remote island where archive clue 190 connects family choices, public stakes, and a precise release-year signal.',
    1979,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    'abc649a3-0a17-5763-9ee0-b03adc96de96',
    'showcase-movie-191',
    'Showcase Movie 191',
    'A deterministic future technology dilemma in an underground archive where archive clue 191 connects family choices, public stakes, and a precise release-year signal.',
    1986,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    'ec373417-d470-5e45-ab30-a32a81a0c788',
    'showcase-movie-192',
    'Showcase Movie 192',
    'A deterministic race against time in a festival weekend where archive clue 192 connects family choices, public stakes, and a precise release-year signal.',
    1993,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    'd6ba0dab-1f77-541e-81fa-38e02992a59e',
    'showcase-movie-193',
    'Showcase Movie 193',
    'A deterministic high-stakes chase in a coastal city where archive clue 193 connects family choices, public stakes, and a precise release-year signal.',
    2000,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    '326dc2ba-8f5d-5344-8285-9502c5737c5b',
    'showcase-movie-194',
    'Showcase Movie 194',
    'A deterministic expedition in an orbital station where archive clue 194 connects family choices, public stakes, and a precise release-year signal.',
    2007,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '38e61ddc-ae14-59c4-bece-659d5d566d82',
    'showcase-movie-195',
    'Showcase Movie 195',
    'A deterministic handcrafted world in a mountain town where archive clue 195 connects family choices, public stakes, and a precise release-year signal.',
    2014,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    'a6482ff8-c95a-5842-9b76-21a93a02c995',
    'showcase-movie-196',
    'Showcase Movie 196',
    'A deterministic mistaken-identity romp in a desert highway where archive clue 196 connects family choices, public stakes, and a precise release-year signal.',
    2021,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    '086b002e-eb26-5ed4-9ebe-cb8ce386e7d6',
    'showcase-movie-197',
    'Showcase Movie 197',
    'A deterministic archival investigation in a rainy capital where archive clue 197 connects family choices, public stakes, and a precise release-year signal.',
    1977,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    'b3583a8a-cf56-516e-be5f-578acccfce9e',
    'showcase-movie-198',
    'Showcase Movie 198',
    'A deterministic personal turning point in a remote island where archive clue 198 connects family choices, public stakes, and a precise release-year signal.',
    1984,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    '108b2161-6dd8-56a9-8a11-be5b8685877e',
    'showcase-movie-199',
    'Showcase Movie 199',
    'A deterministic mythic quest in an underground archive where archive clue 199 connects family choices, public stakes, and a precise release-year signal.',
    1991,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    'f3aff683-19af-5351-baef-9a12615a11ee',
    'showcase-movie-200',
    'Showcase Movie 200',
    'A deterministic haunted mystery in a festival weekend where archive clue 200 connects family choices, public stakes, and a precise release-year signal.',
    1998,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    '53d0efb6-0107-52c8-9f80-292a02d199ad',
    'showcase-movie-201',
    'Showcase Movie 201',
    'A deterministic cold-case puzzle in a coastal city where archive clue 201 connects family choices, public stakes, and a precise release-year signal.',
    2005,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    '37fd7d14-b755-527a-aab7-637ef5f9444e',
    'showcase-movie-202',
    'Showcase Movie 202',
    'A deterministic second-chance relationship in an orbital station where archive clue 202 connects family choices, public stakes, and a precise release-year signal.',
    2012,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '801c9ce3-1e3c-512d-8cc9-0c9b3874b5a7',
    'showcase-movie-203',
    'Showcase Movie 203',
    'A deterministic future technology dilemma in a mountain town where archive clue 203 connects family choices, public stakes, and a precise release-year signal.',
    2019,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    'c43e8c20-71f4-5033-8c9d-6ac88c666c8d',
    'showcase-movie-204',
    'Showcase Movie 204',
    'A deterministic race against time in a desert highway where archive clue 204 connects family choices, public stakes, and a precise release-year signal.',
    1975,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    'be5c30ee-6a44-5c47-bbf8-579d0c7c5214',
    'showcase-movie-205',
    'Showcase Movie 205',
    'A deterministic high-stakes chase in a rainy capital where archive clue 205 connects family choices, public stakes, and a precise release-year signal.',
    1982,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    '83864589-74c6-54f5-8ad5-37cb03d32d16',
    'showcase-movie-206',
    'Showcase Movie 206',
    'A deterministic expedition in a remote island where archive clue 206 connects family choices, public stakes, and a precise release-year signal.',
    1989,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '4ad39328-a8f1-54e0-887c-a81e31b70dda',
    'showcase-movie-207',
    'Showcase Movie 207',
    'A deterministic handcrafted world in an underground archive where archive clue 207 connects family choices, public stakes, and a precise release-year signal.',
    1996,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '386c1fb2-68c6-5f77-95d0-aeb7ebb8c34e',
    'showcase-movie-208',
    'Showcase Movie 208',
    'A deterministic mistaken-identity romp in a festival weekend where archive clue 208 connects family choices, public stakes, and a precise release-year signal.',
    2003,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    '41dbc1f6-a1fd-5497-9051-6769a4882b49',
    'showcase-movie-209',
    'Showcase Movie 209',
    'A deterministic archival investigation in a coastal city where archive clue 209 connects family choices, public stakes, and a precise release-year signal.',
    2010,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    '49f54342-2ff3-5700-b1f8-9df06aab7964',
    'showcase-movie-210',
    'Showcase Movie 210',
    'A deterministic personal turning point in an orbital station where archive clue 210 connects family choices, public stakes, and a precise release-year signal.',
    2017,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    '1542b526-6980-5476-b843-2ab863f6c08d',
    'showcase-movie-211',
    'Showcase Movie 211',
    'A deterministic mythic quest in a mountain town where archive clue 211 connects family choices, public stakes, and a precise release-year signal.',
    2024,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    '782eee57-e434-559a-adeb-43228fbdf1a4',
    'showcase-movie-212',
    'Showcase Movie 212',
    'A deterministic haunted mystery in a desert highway where archive clue 212 connects family choices, public stakes, and a precise release-year signal.',
    1980,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    '016ca567-0ae0-59dc-a4c7-e0725f6ecb4b',
    'showcase-movie-213',
    'Showcase Movie 213',
    'A deterministic cold-case puzzle in a rainy capital where archive clue 213 connects family choices, public stakes, and a precise release-year signal.',
    1987,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    'c531fb1b-6869-5546-a4c8-0ade059f882c',
    'showcase-movie-214',
    'Showcase Movie 214',
    'A deterministic second-chance relationship in a remote island where archive clue 214 connects family choices, public stakes, and a precise release-year signal.',
    1994,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '4e1b7273-3e6f-51bd-b2a9-f1470d9ac934',
    'showcase-movie-215',
    'Showcase Movie 215',
    'A deterministic future technology dilemma in an underground archive where archive clue 215 connects family choices, public stakes, and a precise release-year signal.',
    2001,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '61cabcad-4017-519e-8e6c-f3b81d416fd5',
    'showcase-movie-216',
    'Showcase Movie 216',
    'A deterministic race against time in a festival weekend where archive clue 216 connects family choices, public stakes, and a precise release-year signal.',
    2008,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    '16c80db4-7a4e-53b9-8bec-b937cd2462f8',
    'showcase-movie-217',
    'Showcase Movie 217',
    'A deterministic high-stakes chase in a coastal city where archive clue 217 connects family choices, public stakes, and a precise release-year signal.',
    2015,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    '5d6422f6-de10-57f6-8256-08369c55b94a',
    'showcase-movie-218',
    'Showcase Movie 218',
    'A deterministic expedition in an orbital station where archive clue 218 connects family choices, public stakes, and a precise release-year signal.',
    2022,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '8ab5753e-4662-5cc5-a1be-e990c4cb3383',
    'showcase-movie-219',
    'Showcase Movie 219',
    'A deterministic handcrafted world in a mountain town where archive clue 219 connects family choices, public stakes, and a precise release-year signal.',
    1978,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '21b19639-fa2b-52f7-9fbb-717106ead38a',
    'showcase-movie-220',
    'Showcase Movie 220',
    'A deterministic mistaken-identity romp in a desert highway where archive clue 220 connects family choices, public stakes, and a precise release-year signal.',
    1985,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    '0655cc71-ac50-5450-b9b3-22aa40c62a95',
    'showcase-movie-221',
    'Showcase Movie 221',
    'A deterministic archival investigation in a rainy capital where archive clue 221 connects family choices, public stakes, and a precise release-year signal.',
    1992,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    '22d4d919-d39b-5603-9a16-f8b5e7b87879',
    'showcase-movie-222',
    'Showcase Movie 222',
    'A deterministic personal turning point in a remote island where archive clue 222 connects family choices, public stakes, and a precise release-year signal.',
    1999,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    '89154ae2-7d5c-5e0e-878a-653ce542fc65',
    'showcase-movie-223',
    'Showcase Movie 223',
    'A deterministic mythic quest in an underground archive where archive clue 223 connects family choices, public stakes, and a precise release-year signal.',
    2006,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    'cde18606-fbae-55fb-afef-401d8133e7be',
    'showcase-movie-224',
    'Showcase Movie 224',
    'A deterministic haunted mystery in a festival weekend where archive clue 224 connects family choices, public stakes, and a precise release-year signal.',
    2013,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    'b6451cfd-a72e-55cf-a15e-87cec5c966ed',
    'showcase-movie-225',
    'Showcase Movie 225',
    'A deterministic cold-case puzzle in a coastal city where archive clue 225 connects family choices, public stakes, and a precise release-year signal.',
    2020,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    'ba6230ba-57ae-57c0-9239-f1be2e959c93',
    'showcase-movie-226',
    'Showcase Movie 226',
    'A deterministic second-chance relationship in an orbital station where archive clue 226 connects family choices, public stakes, and a precise release-year signal.',
    1976,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    'c22cf283-e28a-5da7-85e2-3afd4171fcea',
    'showcase-movie-227',
    'Showcase Movie 227',
    'A deterministic future technology dilemma in a mountain town where archive clue 227 connects family choices, public stakes, and a precise release-year signal.',
    1983,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '0eed1cd7-9b8f-5ccb-9f7e-3a477f956727',
    'showcase-movie-228',
    'Showcase Movie 228',
    'A deterministic race against time in a desert highway where archive clue 228 connects family choices, public stakes, and a precise release-year signal.',
    1990,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    '44102472-c4af-536c-a148-767d1110ebe5',
    'showcase-movie-229',
    'Showcase Movie 229',
    'A deterministic high-stakes chase in a rainy capital where archive clue 229 connects family choices, public stakes, and a precise release-year signal.',
    1997,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    '43bfa872-24e1-5a4b-8485-c84af6954c9d',
    'showcase-movie-230',
    'Showcase Movie 230',
    'A deterministic expedition in a remote island where archive clue 230 connects family choices, public stakes, and a precise release-year signal.',
    2004,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '46f1c01b-cfdd-5131-af70-9a2838193fa4',
    'showcase-movie-231',
    'Showcase Movie 231',
    'A deterministic handcrafted world in an underground archive where archive clue 231 connects family choices, public stakes, and a precise release-year signal.',
    2011,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '10384d71-7d6d-5274-ba5a-b2815e18e5c4',
    'showcase-movie-232',
    'Showcase Movie 232',
    'A deterministic mistaken-identity romp in a festival weekend where archive clue 232 connects family choices, public stakes, and a precise release-year signal.',
    2018,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    '40cc16a1-d6f8-5f10-a602-a21316221d52',
    'showcase-movie-233',
    'Showcase Movie 233',
    'A deterministic archival investigation in a coastal city where archive clue 233 connects family choices, public stakes, and a precise release-year signal.',
    2025,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    'cd158d77-f3e3-59a2-bab2-41f913c915ca',
    'showcase-movie-234',
    'Showcase Movie 234',
    'A deterministic personal turning point in an orbital station where archive clue 234 connects family choices, public stakes, and a precise release-year signal.',
    1981,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    'abb02ad8-8834-5ff2-a036-0e41f2152dd6',
    'showcase-movie-235',
    'Showcase Movie 235',
    'A deterministic mythic quest in a mountain town where archive clue 235 connects family choices, public stakes, and a precise release-year signal.',
    1988,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  ),
  (
    '76ee697b-e4ba-50b5-94bb-af3ef8cfd03c',
    'showcase-movie-236',
    'Showcase Movie 236',
    'A deterministic haunted mystery in a desert highway where archive clue 236 connects family choices, public stakes, and a precise release-year signal.',
    1995,
    ARRAY['horror', 'thriller'],
    'Horror',
    '[0.0,0.0,0.0]'
  ),
  (
    'e53fadaf-c608-549f-9f65-b6f371057034',
    'showcase-movie-237',
    'Showcase Movie 237',
    'A deterministic cold-case puzzle in a rainy capital where archive clue 237 connects family choices, public stakes, and a precise release-year signal.',
    2002,
    ARRAY['mystery', 'thriller'],
    'Mystery',
    '[0.0,0.0,0.0]'
  ),
  (
    'a6278e2f-c436-5f42-a3b0-4c027cf330d2',
    'showcase-movie-238',
    'Showcase Movie 238',
    'A deterministic second-chance relationship in a remote island where archive clue 238 connects family choices, public stakes, and a precise release-year signal.',
    2009,
    ARRAY['romance', 'drama'],
    'Romance',
    '[0.0,0.0,0.0]'
  ),
  (
    '0d0574df-d103-573c-bc7e-a5373799d48f',
    'showcase-movie-239',
    'Showcase Movie 239',
    'A deterministic future technology dilemma in an underground archive where archive clue 239 connects family choices, public stakes, and a precise release-year signal.',
    2016,
    ARRAY['sci-fi'],
    'Sci-Fi',
    '[0.0,0.0,0.0]'
  ),
  (
    '7e4d617e-4b03-52ec-9b19-0c53f0112b28',
    'showcase-movie-240',
    'Showcase Movie 240',
    'A deterministic race against time in a festival weekend where archive clue 240 connects family choices, public stakes, and a precise release-year signal.',
    2023,
    ARRAY['thriller', 'crime'],
    'Thriller',
    '[0.0,0.0,0.0]'
  ),
  (
    'b4379f67-0db3-5f12-b275-699653fae54e',
    'showcase-movie-241',
    'Showcase Movie 241',
    'A deterministic high-stakes chase in a coastal city where archive clue 241 connects family choices, public stakes, and a precise release-year signal.',
    1979,
    ARRAY['action', 'adventure'],
    'Action',
    '[0.0,0.0,0.0]'
  ),
  (
    'eb924240-759b-5071-8246-90696326f2a7',
    'showcase-movie-242',
    'Showcase Movie 242',
    'A deterministic expedition in an orbital station where archive clue 242 connects family choices, public stakes, and a precise release-year signal.',
    1986,
    ARRAY['adventure', 'family'],
    'Adventure',
    '[0.0,0.0,0.0]'
  ),
  (
    '0a208749-4f37-5a5d-b715-60d0e05102b9',
    'showcase-movie-243',
    'Showcase Movie 243',
    'A deterministic handcrafted world in a mountain town where archive clue 243 connects family choices, public stakes, and a precise release-year signal.',
    1993,
    ARRAY['animation', 'family'],
    'Animation',
    '[0.0,0.0,0.0]'
  ),
  (
    '7e0cf065-f703-55e7-9ea2-b5a43d70f64b',
    'showcase-movie-244',
    'Showcase Movie 244',
    'A deterministic mistaken-identity romp in a desert highway where archive clue 244 connects family choices, public stakes, and a precise release-year signal.',
    2000,
    ARRAY['comedy'],
    'Comedy',
    '[0.0,0.0,0.0]'
  ),
  (
    '206d8fc4-a73d-5895-a486-1e4be08bcd86',
    'showcase-movie-245',
    'Showcase Movie 245',
    'A deterministic archival investigation in a rainy capital where archive clue 245 connects family choices, public stakes, and a precise release-year signal.',
    2007,
    ARRAY['documentary'],
    'Documentary',
    '[0.0,0.0,0.0]'
  ),
  (
    'cb773cc0-7d88-5078-b2f2-9706d81afb51',
    'showcase-movie-246',
    'Showcase Movie 246',
    'A deterministic personal turning point in a remote island where archive clue 246 connects family choices, public stakes, and a precise release-year signal.',
    2014,
    ARRAY['drama'],
    'Drama',
    '[0.0,0.0,0.0]'
  ),
  (
    '4084e709-fa54-5b64-b57c-d28a4b6b9eba',
    'showcase-movie-247',
    'Showcase Movie 247',
    'A deterministic mythic quest in an underground archive where archive clue 247 connects family choices, public stakes, and a precise release-year signal.',
    2021,
    ARRAY['fantasy', 'adventure'],
    'Fantasy',
    '[0.0,0.0,0.0]'
  )
ON CONFLICT (slug) DO UPDATE SET
  id = EXCLUDED.id,
  title = EXCLUDED.title,
  overview = EXCLUDED.overview,
  release_year = EXCLUDED.release_year,
  genres = EXCLUDED.genres,
  primary_genre = EXCLUDED.primary_genre,
  embedding = EXCLUDED.embedding,
  updated_at = now()
WHERE
  movies.id IS DISTINCT FROM EXCLUDED.id
  OR movies.title IS DISTINCT FROM EXCLUDED.title
  OR movies.overview IS DISTINCT FROM EXCLUDED.overview
  OR movies.release_year IS DISTINCT FROM EXCLUDED.release_year
  OR movies.genres IS DISTINCT FROM EXCLUDED.genres
  OR movies.primary_genre IS DISTINCT FROM EXCLUDED.primary_genre
  OR movies.embedding IS DISTINCT FROM EXCLUDED.embedding;
