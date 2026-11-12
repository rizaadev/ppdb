UPDATE tenants
SET theme_config = jsonb_build_object(
    'tagline', 'Pondok Pesantren Al-Hamid Cinta Mulya — Pendidikan Islam Berkarakter, Unggul dalam Ilmu & Akhlakul Karimah',
    'primary_color', '#047857'
)
WHERE slug = 'alhamid-cintamulya';

SELECT slug, name, theme_config FROM tenants WHERE slug = 'alhamid-cintamulya';

