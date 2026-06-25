-- A catalog of selectable exercises for the lift logger (reference data, seeded here).
CREATE TABLE exercise_library (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name      text    NOT NULL,
    muscle    text    NOT NULL,            -- Chest, Back, Legs, Shoulders, Arms, Core
    equipment text    NOT NULL DEFAULT '', -- Barbell, Dumbbell, Machine, Cable, Bodyweight
    position  integer NOT NULL DEFAULT 0
);
CREATE INDEX idx_exercise_library_muscle ON exercise_library (muscle, position);
CREATE UNIQUE INDEX idx_exercise_library_name ON exercise_library (name);

INSERT INTO exercise_library (name, muscle, equipment, position) VALUES
  ('Barbell Bench Press',     'Chest',     'Barbell',    0),
  ('Incline Bench Press',     'Chest',     'Barbell',    1),
  ('Dumbbell Bench Press',    'Chest',     'Dumbbell',   2),
  ('Cable Fly',               'Chest',     'Cable',      3),
  ('Chest Dip',               'Chest',     'Bodyweight', 4),
  ('Push-Up',                 'Chest',     'Bodyweight', 5),
  ('Deadlift',                'Back',      'Barbell',    0),
  ('Pull-Up',                 'Back',      'Bodyweight', 1),
  ('Barbell Row',             'Back',      'Barbell',    2),
  ('Lat Pulldown',            'Back',      'Cable',      3),
  ('Seated Cable Row',        'Back',      'Cable',      4),
  ('Face Pull',               'Back',      'Cable',      5),
  ('Back Squat',              'Legs',      'Barbell',    0),
  ('Front Squat',             'Legs',      'Barbell',    1),
  ('Leg Press',               'Legs',      'Machine',    2),
  ('Romanian Deadlift',       'Legs',      'Barbell',    3),
  ('Leg Extension',           'Legs',      'Machine',    4),
  ('Lying Leg Curl',          'Legs',      'Machine',    5),
  ('Walking Lunge',           'Legs',      'Dumbbell',   6),
  ('Standing Calf Raise',     'Legs',      'Machine',    7),
  ('Overhead Press',          'Shoulders', 'Barbell',    0),
  ('Dumbbell Shoulder Press', 'Shoulders', 'Dumbbell',   1),
  ('Lateral Raise',           'Shoulders', 'Dumbbell',   2),
  ('Rear Delt Fly',           'Shoulders', 'Dumbbell',   3),
  ('Arnold Press',            'Shoulders', 'Dumbbell',   4),
  ('Barbell Curl',            'Arms',      'Barbell',    0),
  ('Dumbbell Curl',           'Arms',      'Dumbbell',   1),
  ('Hammer Curl',             'Arms',      'Dumbbell',   2),
  ('Triceps Pushdown',        'Arms',      'Cable',      3),
  ('Skullcrusher',            'Arms',      'Barbell',    4),
  ('Close-Grip Bench Press',  'Arms',      'Barbell',    5),
  ('Plank',                   'Core',      'Bodyweight', 0),
  ('Hanging Leg Raise',       'Core',      'Bodyweight', 1),
  ('Cable Crunch',            'Core',      'Cable',      2),
  ('Russian Twist',           'Core',      'Bodyweight', 3),
  ('Ab Wheel Rollout',        'Core',      'Bodyweight', 4);
